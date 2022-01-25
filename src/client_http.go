package main

import (
	"io"
	"net"
	"net/http"
)

func (client *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if client.IdleTimeout.Seconds() > 0 {
		client.idleEventC <- struct{}{}
	}

	// rewrite target
	r.Close = false
	r.URL.Host = client.ep.ProxyAddress
	r.RequestURI = ""

	if r.Method == http.MethodConnect {
		handleTunneling(client.Dial, w, r)
	} else {
		handleHTTP(client.httpClient, w, r)
	}
}

type Dialer func(network, address string) (net.Conn, error)

func handleTunneling(dialer Dialer, w http.ResponseWriter, r *http.Request) {
	dest_conn, err := dialer("tcp", r.URL.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	client_conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	go transfer(dest_conn, client_conn)
	go transfer(client_conn, dest_conn)
}

func transfer(destination io.WriteCloser, source io.ReadCloser) {
	defer destination.Close()
	defer source.Close()
	io.Copy(destination, source)
}

func handleHTTP(client *http.Client, w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	req.RequestURI = ""

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range w.Header() {
		for _, v := range vv {
			resp.Header.Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
