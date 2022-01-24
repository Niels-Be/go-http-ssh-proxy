HTTP-SSH-Proxy
==============

A Simple Proxy that routes requests through ssh tunnels based on Host header.
It allows you to setup a single proxy server to access multiple private services on remote networks.

This is useful in scenarios where you only have SSH access to a Server but want to access a private HTTP Endpoint.
For example a central Prometheus server that scrapes multiple node-exportes without having to expose the node-exporter port publicly.


### Install
```
go install http-ssh-proxy
```

### Example Config
config.yml
```yml
# Bind: localhost:8082 # default listen address
# Set global ssh defaults
DefaultSSHKey: ~/.ssh/id_rsa
DefaultUsername: anon
IdleTimeout: 120s # close idle connections after 120sec. 0 to disable
Endpoints:
  # Imagine a service running on ingres.public.com that binds to 127.0.0.1:80
  # Expose service on ingres.public.com:80 as http://test.network.local/
  - VHostname: test.network.local
    SSHHostname: ingres.public.com
    # SSHPort: 22 # default
    # SSHConnectTimeout: 15s # default
    # SSHKey: # defaults to .DefaultSSHKey
    # Username: # defaults to .DefaultUsername
    # ProxyAddress: localhost:80 # default
  # jumpbox.public.com is a public server that is connected to a private network
  # Here you expose other.network.local:9200 as if it was directly publicly reachable
  - VHostname: other.network.local:9200
    SSHHostname: jumpbox.public.com
    ProxyAddress: other.network.local:9200
# Enable more verbose output
Debug: true
```

Example Curl usage:
```bash
# Via Host Header
curl -H "Host: test.network.local" http://localhost:8082/
# Via Proxy Parameter
curl --proxy http://localhost:8082/ http://other.network.local:9200/
# Via Environment Variable
export http_proxy=http://localhost:8082/
curl http://test.network.local/
```


### Security
Please not that this essentially punches a big hohle in you Firewall setup.
Because you can expose anything on the local network of the target server.
Make sure to only configure safe routes and do not expose this proxy on the internet.

To be safe you should create a separate SSH user without a login shell that is only used for this proxy.
