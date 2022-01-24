package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/yaml.v2"
)

type Configuration struct {
	Bind string `yaml:"Bind"`

	DefaultSSHKey   string `yaml:"DefaultSSHKey"`
	DefaultUsername string `yaml:"DefaultUsername"`

	Endpoints []*RemoteEndpoint `yaml:"Endpoints"`

	Debug bool `yaml:"Debug"`
}

type RemoteEndpoint struct {
	VHostname string `yaml:"VHostname"`

	SSHHostname string `yaml:"SSHHostname"`
	SSHPort     int    `yaml:"SSHPort"`
	Username    string `yaml:"Username"`
	SSHKey      string `yaml:"SSHKey"`

	ProxyAddress string `yaml:"ProxyAddress"`
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
	}
	return b
}

func main() {

	config := parseConfig("./config.yml")
	log.Printf("Config: %v", config)

	proxy := NewProxy(&config)

	server := &http.Server{Addr: config.Bind, Handler: &proxy}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Printf("Closing")
		server.Close()
		proxy.Close()
	}()

	err := server.ListenAndServe()
	if err != nil {
		if err.Error() != "http: Server closed" {
			log.Fatalf("Server Error: %s", err.Error())
		}
	}
}
