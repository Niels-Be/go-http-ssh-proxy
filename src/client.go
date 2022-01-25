package main

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	ep         *RemoteEndpoint
	sshConfig  ssh.ClientConfig
	sshClient  *ssh.Client
	httpClient *http.Client
	mtx        sync.Mutex
	ctx        context.Context
	idleEventC chan struct{}

	IdleTimeout time.Duration
	Debug       bool
}

func getKeyAuth(path string) (ssh.AuthMethod, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(buf)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

func NewClient(ctx context.Context, ep *RemoteEndpoint) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	auth, err := getKeyAuth(ep.SSHKey)
	if err != nil {
		cancel()
		return nil, err
	}
	client := Client{
		ep: ep,
		sshConfig: ssh.ClientConfig{
			User: ep.Username,
			Auth: []ssh.AuthMethod{auth},
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				return nil
			},
			Timeout: ep.SSHConnectTimeout,
		},
		ctx:        ctx,
		idleEventC: make(chan struct{}, 10),
	}
	client.httpClient = &http.Client{
		Transport: &http.Transport{
			Dial:            client.Dial,
			MaxIdleConns:    2,
			IdleConnTimeout: 60 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// never redirect
			return http.ErrUseLastResponse
		},
	}

	go func() {
		<-ctx.Done()
		cancel()
		client.mtx.Lock()
		defer client.mtx.Unlock()
		client.closeSshClient()
	}()

	return &client, nil
}

func (client *Client) connect() error {
	sshClient, err := ssh.Dial("tcp", client.ep.SSHAddr(), &client.sshConfig)
	if err != nil {
		log.Printf("SSH connection to %s failed: %v", client.ep.SSHAddr(), err)
		return err
	}

	client.sshClient = sshClient
	log.Printf("SSH connection to %s established", client.ep.SSHAddr())

	ctx, cancel := context.WithCancel(client.ctx)
	go func() {
		sshClient.Wait()
		log.Printf("SSH connection to %s closed", client.ep.SSHAddr())
		cancel()
	}()
	if client.IdleTimeout.Seconds() > 0 {
		go func() {
			timer := time.NewTimer(client.IdleTimeout)
			for {
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
					if client.Debug {
						log.Printf("Close SSH connection to %s by idle timeout", client.ep.SSHAddr())
					}
					sshClient.Close()
					return
				case <-client.idleEventC:
					if !timer.Stop() {
						<-timer.C
					}
					timer.Reset(client.IdleTimeout)
				}
			}
		}()
	}

	return nil
}

// establishes a TCP connection through SSH
func (client *Client) Dial(network, address string) (net.Conn, error) {
	client.mtx.Lock()
	defer client.mtx.Unlock()

	retried := false

retry:
	if client.sshClient == nil {
		if err := client.connect(); err != nil {
			return nil, err
		}
	}

	conn, err := client.sshClient.Dial(network, address)

	if err != nil && !retried && (err == io.EOF || !client.IsAlive()) {
		// ssh connection broken
		client.closeSshClient()

		retried = true
		goto retry
	}

	if err != nil {
		log.Printf("TCP forwarding via %s to %s failed: %s", client.ep.SSHAddr(), address, err)
	} else if client.Debug {
		log.Printf("TCP forwarding via %s to %s established", client.ep.SSHAddr(), address)
	}

	return conn, err
}

// checks if the SSH client is still alive by sending a keep alive request.
func (client *Client) IsAlive() bool {
	if client.sshClient == nil {
		return false
	}
	_, _, err := client.sshClient.Conn.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

// call this only if you have a mtx.Lock()
func (client *Client) closeSshClient() {
	client.httpClient.Transport.(*http.Transport).CloseIdleConnections()
	if client.sshClient != nil {
		client.sshClient.Close()
		client.sshClient = nil
	}
}
