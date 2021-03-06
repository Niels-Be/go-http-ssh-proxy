package main

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Configuration struct {
	Bind string `yaml:"Bind"`

	DefaultSSHKey   string `yaml:"DefaultSSHKey"`
	DefaultUsername string `yaml:"DefaultUsername"`

	// Time in idle after a SSH connection is closed. 0 for infinite
	IdleTimeout time.Duration `yaml:"IdleTimeout"`
	// directly proxy hosts not found in config
	ProxyFallback bool `yaml:"ProxyFallback"`

	Endpoints []*RemoteEndpoint `yaml:"Endpoints"`

	Debug bool `yaml:"Debug"`
}

type RemoteEndpoint struct {
	VHostname string `yaml:"VHostname"`

	SSHHostname       string        `yaml:"SSHHostname"`
	SSHPort           int           `yaml:"SSHPort"`
	Username          string        `yaml:"Username"`
	SSHKey            string        `yaml:"SSHKey"`
	SSHConnectTimeout time.Duration `yaml:"SSHConnectTimeout"`

	ProxyAddress string `yaml:"ProxyAddress"`
}

func (ep *RemoteEndpoint) SSHAddr() string {
	return ep.SSHHostname + ":" + strconv.Itoa(ep.SSHPort)
}

func parseConfig(filename string) Configuration {
	var b = Configuration{
		Bind: "localhost:8082",
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("cannot read config: %v", err)
	}

	err = yaml.Unmarshal(data, &b)
	if err != nil {
		log.Fatalf("cannot unmarshal data: %v", err)
	}

	for _, ep := range b.Endpoints {
		if ep.Username == "" {
			ep.Username = b.DefaultUsername
		}
		if ep.SSHKey == "" {
			ep.SSHKey = b.DefaultSSHKey
		}
		if ep.ProxyAddress == "" {
			ep.ProxyAddress = "localhost:80"
		}
		if ep.SSHPort == 0 {
			ep.SSHPort = 22
		}
		if ep.SSHConnectTimeout == 0 {
			ep.SSHConnectTimeout = 15 * time.Second
		}
	}
	return b
}

func main() {

	config := parseConfig("./config.yml")
	log.Printf("Config: %v", config)

	ctx, cancel := context.WithCancel(context.Background())

	proxy := NewProxy(ctx, &config)

	server := &http.Server{Addr: config.Bind, Handler: &proxy}

	c := make(chan os.Signal)

	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Printf("Closing")
		server.Close()
		cancel()
	}()

	err := server.ListenAndServe()
	if err != nil {
		if err.Error() != "http: Server closed" {
			log.Fatalf("Server Error: %s", err.Error())
		}
	}
	<-ctx.Done()
}
