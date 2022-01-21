HTTP-SSH-Proxy
==============

A Simple Reverse Proxy that routes requests through ssh tunnels based on Host header.



### Example Config
```yml
DefaultSSHKey: ~/.ssh/id_rsa
DefaultUsername: anon
Endpoints:
  - VHostname: test.local.net
    SSHHostname: test.example.net
  - VHostname: other.local.net
    SSHHostname: other.example.net
Debug: true
```
```bash
curl -H "Host: test.local.net" http://localhost:8082/
```