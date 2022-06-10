# Clash for SagerNet

### Changes

* Add support for Shadowsocks 2022 ciphers

```yaml
proxies:
  - name: "shadowsocks"
    type: ss
    server: server
    port: 443
    cipher: 2022-blake3-aes-128-gcm
    password: "<psk>"
```

* Refactor vmess:
    - Add support for packetaddr/masking/authenticated-length protocol
    - Add support for zero/aes-128-cfb cipher

```yaml
proxies:
  - name: "vmess"
    type: vmessk
    packet-addr: true
    authenticated-length: true
```