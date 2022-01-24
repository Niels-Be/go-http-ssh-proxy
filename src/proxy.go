package main

import (
	"context"
	"http-ssh-proxy/src/sshtun"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
)

type proxyEntry struct {
	tun         *sshtun.SSHTun
	proxy       *httputil.ReverseProxy
	timeoutChan *chan struct{}
}

func (p *proxyEntry) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.timeoutChan != nil {
		*p.timeoutChan <- struct{}{}
	}
	if r.Method == http.MethodConnect {
		p.handleTunneling(w, r)
	} else {
		p.proxy.ServeHTTP(w, r)
	}
}

func (p *proxyEntry) handleTunneling(w http.ResponseWriter, r *http.Request) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	client_conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	defer client_conn.Close()
	w.WriteHeader(http.StatusOK)
	p.tun.Forward(client_conn)
}

type Proxy struct {
	openTunnels map[string]proxyEntry
	config      *Configuration
}

func NewProxy(config *Configuration) Proxy {
	return Proxy{
		openTunnels: map[string]proxyEntry{},
		config:      config,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.config.Debug {
		log.Printf("Proxy: %s %s %s", r.Method, r.Host, r.URL.String())
	}

	// try an open tunnel
	entry, ok := p.openTunnels[r.Host]
	if ok {
		entry.ServeHTTP(w, r)
		return
	}

	// try to create a new tunnel
	for _, ep := range p.config.Endpoints {
		if ep.VHostname == r.Host {
			tun, err := p.createTunnel(ep)
			if err != nil {
				log.Printf("Could not setup tunnel for %s %s", r.Host, err.Error())
				w.WriteHeader(500)
				return
			}
			tun.ServeHTTP(w, r)
			return
		}
	}

	// fail with BadGateway
	log.Printf("Host %s Not Found", r.Host)
	w.WriteHeader(502)

}

func (p *Proxy) Close() {
	for _, e := range p.openTunnels {
		e.tun.Stop()
	}
}

func (p *Proxy) createTunnel(ep *RemoteEndpoint) (*proxyEntry, error) {
	addr, err := net.ResolveTCPAddr("tcp4", ep.ProxyAddress)
	if err != nil {
		return nil, err
	}

	tun := sshtun.New(0, ep.SSHHostname, addr.Port)
	tun.SetUser(ep.Username)
	tun.SetDebug(p.config.Debug)
	tun.SetKeyFile(ep.SSHKey)
	tun.SetPort(ep.SSHPort)
	tun.SetRemoteHost(addr.IP.String())
	if ep.SSHConnectTimeout != nil {
		tun.SetTimeout(*ep.SSHConnectTimeout)
	}

	if err := tun.Start(); err != nil {
		return nil, err
	}

	go func() {
		err := tun.Wait()
		if err != nil {
			log.Printf("SSH Tunnel Error: %s", err.Error())
		} else if p.config.Debug {
			log.Printf("SSH Tunnel Closed")
		}
		delete(p.openTunnels, ep.VHostname)
	}()

	var timeoutChan *chan struct{}
	if p.config.IdleTimeout.Seconds() > 0 {
		c := make(chan struct{})
		timeoutChan = &c
		go func() {
			ctx, cancel := context.WithTimeout(*tun.GetContext(), p.config.IdleTimeout)
			for {
				select {
				case <-ctx.Done():
					close(*timeoutChan)
					if ctx.Err().Error() == "context deadline exceeded" {
						if p.config.Debug {
							log.Printf("Closing connection to %s by Idle timeout", ep.SSHHostname)
						}
						tun.Stop()
					}
					cancel()
					return
				case <-*timeoutChan:
					// reset timeout
					cancel()
					ctx, cancel = context.WithTimeout(*tun.GetContext(), p.config.IdleTimeout)
				}
			}
		}()
	}

	url := url.URL{
		Scheme: "http",
		Host:   "localhost:" + strconv.Itoa(tun.GetLocalPort()),
	}

	entry := proxyEntry{
		tun:         tun,
		proxy:       httputil.NewSingleHostReverseProxy(&url),
		timeoutChan: timeoutChan,
	}
	p.openTunnels[ep.VHostname] = entry
	return &entry, nil
}
