
# Mesh Tunnel Production Starter

## Features
- Self-hosted relay server
- Cloudflare Tunnel support
- QUIC P2P transport
- Login/Register
- Persistent virtual IP
- Gio Android UI
- gomobile APK build

## Server Install

```bash
cd server
go mod tidy
go run server.go
```

## Cloudflare Tunnel

Install cloudflared then:

```bash
cloudflared tunnel --url http://localhost:8080
```

Use:

```txt
wss://xxxxx.trycloudflare.com/ws
```

## Build APK

```bash
go install golang.org/x/mobile/cmd/gomobile@latest

gomobile init

cd android

go mod tidy

gomobile build -target=android
```

APK output:

```txt
app.apk
```
