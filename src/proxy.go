package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"
)

var DefaultDialer = net.Dialer{Timeout: 15 * time.Second}

type Proxy struct {
	clients map[string]*Client
	config  *Configuration
	ctx     context.Context
}

func NewProxy(ctx context.Context, config *Configuration) Proxy {
	return Proxy{
		clients: map[string]*Client{},
		config:  config,
		ctx:     ctx,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.config.Debug {
		log.Printf("Proxy: %s %s %s", r.Method, r.Host, r.URL.String())
	}

	client, err := p.getClient(r.Host)
	if err != nil {
		log.Printf("Could not setup tunnel for %s %s", r.Host, err.Error())
		w.WriteHeader(500)
	}
	if client != nil {
		client.ServeHTTP(w, r)
		return
	}

	if p.config.ProxyFallback {
		if r.Method == http.MethodConnect {
			handleTunneling(DefaultDialer.Dial, w, r)
		} else {
			handleHTTP(http.DefaultClient, w, r)
		}
	} else {
		// fail with BadGateway
		log.Printf("Host %s Not Found", r.Host)
		w.WriteHeader(502)
	}
}

func (p *Proxy) getClient(hostname string) (*Client, error) {
	entry, ok := p.clients[hostname]
	if ok {
		return entry, nil
	}

	// try to create a new tunnel
	for _, ep := range p.config.Endpoints {
		if ep.VHostname == hostname {
			c, err := NewClient(p.ctx, ep)
			if c != nil {
				c.IdleTimeout = p.config.IdleTimeout
				p.clients[hostname] = c
			}
			return c, err
		}
	}
	return nil, nil
}
