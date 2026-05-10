package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type Packet struct {
	Type       string `json:"type"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	VirtualIP  string `json:"virtual_ip,omitempty"`
	PublicAddr string `json:"public_addr,omitempty"`
}

type ForwardRule struct {
	TargetAddr string
	Protocol   string
	LocalPort  int
	RemotePort int
}

var (
	usernameEditor widget.Editor
	passwordEditor widget.Editor
	serverEditor   widget.Editor

	connectBtn widget.Clickable

	status = "Disconnected"

	virtualIP string

	ws *websocket.Conn
)

func generateTLSConfig() *tls.Config {

	key, _ := rsa.GenerateKey(rand.Reader, 2048)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"MeshTunnel"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
	}

	certDER, _ := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&key.PublicKey,
		key,
	)

	keyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		},
	)

	certPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certDER,
		},
	)

	tlsCert, _ := tls.X509KeyPair(certPEM, keyPEM)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"mesh"},
	}
}

func startQUICServer() {

	listener, err := quic.ListenAddr(
		":9999",
		generateTLSConfig(),
		nil,
	)

	if err != nil {
		log.Println(err)
		return
	}

	for {

		conn, err := listener.Accept(context.Background())
		if err != nil {
			continue
		}

		go handleConn(conn)
	}
}

func handleConn(conn quic.Connection) {

	for {

		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}

		go handleStream(stream)
	}
}

func handleStream(stream quic.Stream) {

	target, err := net.Dial("tcp", "127.0.0.1:25565")
	if err != nil {
		return
	}

	go io.Copy(target, stream)
	go io.Copy(stream, target)
}

func login() {

	conn, _, err := websocket.DefaultDialer.Dial(
		serverEditor.Text(),
		nil,
	)

	if err != nil {
		status = err.Error()
		return
	}

	ws = conn

	localAddr := "127.0.0.1:9999"

	conn.WriteJSON(Packet{
		Type:       "login",
		Username:   usernameEditor.Text(),
		Password:   passwordEditor.Text(),
		PublicAddr: localAddr,
	})

	var resp Packet

	conn.ReadJSON(&resp)

	if resp.Type == "login_success" {

		status = "Connected"

		virtualIP = resp.VirtualIP

	} else {

		status = "Login Failed"
	}
}

func ui() {

	go func() {

		w := new(app.Window)

		w.Option(
			app.Title("Mesh Tunnel"),
			app.Size(unit.Dp(420), unit.Dp(760)),
		)

		th := material.NewTheme()
		th.Shaper = text.NewShaper(
			text.WithCollection(gofont.Collection()),
		)

		var ops op.Ops

		for {

			e := w.Event()

			switch e := e.(type) {

			case app.DestroyEvent:
				os.Exit(0)

			case system.FrameEvent:

				gtx := app.NewContext(&ops, e)

				if connectBtn.Clicked(gtx) {
					go login()
				}

				layout.UniformInset(unit.Dp(20)).Layout(
					gtx,
					func(gtx layout.Context) layout.Dimensions {

						return layout.Flex{
							Axis: layout.Vertical,
						}.Layout(

							gtx,

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.H3(
									th,
									"Mesh Tunnel",
								).Layout(gtx)
							}),

							layout.Rigid(layout.Spacer{
								Height: unit.Dp(20),
							}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {

								return material.Editor(
									th,
									&usernameEditor,
									"Username",
								).Layout(gtx)
							}),

							layout.Rigid(layout.Spacer{
								Height: unit.Dp(12),
							}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {

								return material.Editor(
									th,
									&passwordEditor,
									"Password",
								).Layout(gtx)
							}),

							layout.Rigid(layout.Spacer{
								Height: unit.Dp(12),
							}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {

								return material.Editor(
									th,
									&serverEditor,
									"wss://your-tunnel/ws",
								).Layout(gtx)
							}),

							layout.Rigid(layout.Spacer{
								Height: unit.Dp(20),
							}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {

								return material.Button(
									th,
									&connectBtn,
									"LOGIN",
								).Layout(gtx)
							}),

							layout.Rigid(layout.Spacer{
								Height: unit.Dp(20),
							}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {

								return material.Body1(
									th,
									"Status: "+status,
								).Layout(gtx)
							}),

							layout.Rigid(layout.Spacer{
								Height: unit.Dp(12),
							}.Layout),

							layout.Rigid(func(gtx layout.Context) layout.Dimensions {

								return material.Body1(
									th,
									"Virtual IP: "+virtualIP,
								).Layout(gtx)
							}),
						)
					},
				)

				e.Frame(gtx.Ops)
			}
		}
	}()

	app.Main()
}

func main() {

	serverEditor.SetText(
		"ws://127.0.0.1:8080/ws",
	)

	go startQUICServer()

	ui()
}