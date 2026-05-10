package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
)

// GANTI DENGAN URL CLOUDFLARE ANDA
const ServerWSS = "wss://your-tunnel-name.trycloudflare.com/ws"

type UserInfo struct {
	Username  string `json:"username"`
	VirtualIP string `json:"virtual_ip"`
	IsOnline  bool   `json:"is_online"`
}

type AppState struct {
	User     UserInfo
	LoggedIn bool
	Users    []UserInfo
	MenuOpen bool
	View     string 
	TLS      *tls.Config
	Mutex    sync.Mutex
}

func main() {
	state := &AppState{
		TLS:  &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"mesh"}},
		View: "forward",
	}

	// Load Session Persistent
	if b, err := os.ReadFile("session.json"); err == nil {
		json.Unmarshal(b, &state.User)
		state.LoggedIn = true
		go state.startReceiver()
		go state.syncUserList()
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("MeshTunnel"), app.Size(unit.Dp(400), unit.Dp(800)))
		
		// API Gio Terbaru: NewTheme tidak butuh argumen
		th := material.NewTheme()
		th.Shaper = gofont.Collection()[0].Font.Typeface 

		var ops op.Ops
		var userEd, passEd, ipEd, lpEd, rpEd widget.Editor
		var loginBtn, regBtn, menuBtn, navFwd, navUsers, logoutBtn, addBtn widget.Clickable
		var list widget.List

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case system.DestroyEvent:
				os.Exit(0)
			case system.FrameEvent:
				gtx := layout.NewContext(&ops, e)

				if !state.LoggedIn {
					// --- LAYAR AUTH ---
					if loginBtn.Clicked(gtx) { state.authAction("LOGIN", userEd.Text(), passEd.Text()) }
					if regBtn.Clicked(gtx) { state.authAction("REGISTER", userEd.Text(), passEd.Text()) }
					
					layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(material.H4(th, "MeshTunnel").Layout),
							layout.Rigid(material.Editor(th, &userEd, "Username").Layout),
							layout.Rigid(material.Editor(th, &passEd, "Password").Layout),
							layout.Rigid(material.Button(th, &loginBtn, "LOGIN").Layout),
							layout.Rigid(material.Button(th, &regBtn, "REGISTER").Layout),
						)
					})
				} else {
					// --- DASHBOARD DENGAN DRAWER ---
					if menuBtn.Clicked(gtx) { state.MenuOpen = !state.MenuOpen }
					if navFwd.Clicked(gtx) { state.View = "forward"; state.MenuOpen = false }
					if navUsers.Clicked(gtx) { state.View = "users"; state.MenuOpen = false }
					if logoutBtn.Clicked(gtx) { state.logout() }

					layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{}.Layout(gtx,
								layout.Rigid(material.Button(th, &menuBtn, "≡").Layout),
								layout.Flexed(1, material.H6(th, " MeshTunnel").Layout),
							)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Stack{}.Layout(gtx,
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									if state.View == "forward" {
										return drawForwardView(gtx, th, state, &addBtn, &ipEd, &lpEd, &rpEd)
									}
									return drawUserListView(gtx, th, state, &list)
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									if !state.MenuOpen { return layout.Dimensions{} }
									gtx.Constraints.Max.X = gtx.Dp(unit.Dp(260))
									// Drawer BG
									return material.List(th, &widget.List{Axis: layout.Vertical}).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(material.Button(th, &navFwd, "Port Forwarding").Layout),
											layout.Rigid(material.Button(th, &navUsers, "Daftar User").Layout),
											layout.Rigid(material.Button(th, &logoutBtn, "Logout").Layout),
										)
									})
								}),
							)
						}),
					)
				}
				e.Frame(gtx.Ops)
			}
		}
	}()
	app.Main()
}

// --- LOGIC SIGNALLING & ENGINE ---

func (s *AppState) authAction(op, user, pass string) {
	conn, _, err := websocket.DefaultDialer.Dial(ServerWSS, nil)
	if err != nil { return }
	defer conn.Close()

	conn.WriteJSON(map[string]string{"type": op, "username": user, "password": pass})
	var res map[string]interface{}
	conn.ReadJSON(&res)

	if res["status"] == "ok" {
		userData, _ := json.Marshal(res["user"])
		json.Unmarshal(userData, &s.User)
		s.LoggedIn = true
		os.WriteFile("session.json", userData, 0644)
		go s.startReceiver()
		go s.syncUserList()
	}
}

func (s *AppState) syncUserList() {
	conn, _, _ := websocket.DefaultDialer.Dial(ServerWSS, nil)
	for {
		conn.WriteJSON(map[string]string{"type": "GET_USERS"})
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil { break }
		if msg["type"] == "USER_LIST" {
			s.Mutex.Lock()
			b, _ := json.Marshal(msg["users"])
			json.Unmarshal(b, &s.Users)
			s.Mutex.Unlock()
		}
	}
}

func (s *AppState) logout() {
	s.LoggedIn = false
	os.Remove("session.json")
}

func (s *AppState) startReceiver() {
	l, _ := quic.ListenAddr(":9999", s.TLS, &quic.Config{EnableDatagrams: true})
	for {
		sess, _ := l.Accept(context.Background())
		go func(conn quic.Connection) {
			for {
				stream, _ := conn.AcceptStream(context.Background())
				go func() {
					var m struct{ Proto string; Port int }
					json.NewDecoder(stream).Decode(&m)
					dest, _ := net.Dial(m.Proto, fmt.Sprintf("127.0.0.1:%d", m.Port))
					go io.Copy(dest, stream)
					io.Copy(stream, dest)
				}()
			}
		}(sess)
	}
}

func drawForwardView(gtx layout.Context, th *material.Theme, s *AppState, add *widget.Clickable, ip, lp, rp *widget.Editor) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(material.H6(th, "Port Forwarding (Maks 10)").Layout),
		layout.Rigid(material.Editor(th, ip, "Target VIP").Layout),
		layout.Rigid(material.Editor(th, lp, "Local Port").Layout),
		layout.Rigid(material.Editor(th, rp, "Remote Port").Layout),
		layout.Rigid(material.Button(th, add, "Tambah Tunnel").Layout),
	)
}

func drawUserListView(gtx layout.Context, th *material.Theme, s *AppState, l *widget.List) layout.Dimensions {
	return material.List(th, l).Layout(gtx, len(s.Users), func(gtx layout.Context, i int) layout.Dimensions {
		u := s.Users[i]
		st := "Offline"
		if u.IsOnline { st = "Online" }
		return material.Body1(th, fmt.Sprintf("%s (%s) - %s", u.Username, u.VirtualIP, st)).Layout(gtx)
	})
}
