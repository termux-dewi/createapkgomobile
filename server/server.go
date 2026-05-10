
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type User struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	VirtualIP  string `json:"virtual_ip"`
	PublicAddr string `json:"public_addr"`
}

type Packet struct {
	Type       string `json:"type"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	VirtualIP  string `json:"virtual_ip,omitempty"`
	PublicAddr string `json:"public_addr,omitempty"`
	Peers      []User `json:"peers,omitempty"`
}

var (
	usersFile = "users.json"
	users     = map[string]User{}
	clients   = map[*websocket.Conn]string{}
	mutex     sync.Mutex

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func randomIP() string {
	return fmt.Sprintf("10.0.0.%d", rand.Intn(200)+2)
}

func loadUsers() {
	data, err := os.ReadFile(usersFile)
	if err == nil {
		json.Unmarshal(data, &users)
	}
}

func saveUsers() {
	data, _ := json.MarshalIndent(users, "", "  ")
	os.WriteFile(usersFile, data, 0644)
}

func broadcastPeers() {
	peerList := []User{}
	for _, username := range clients {
		peerList = append(peerList, users[username])
	}

	msg := Packet{
		Type:  "peers",
		Peers: peerList,
	}

	data, _ := json.Marshal(msg)

	for conn := range clients {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		var msg Packet

		if err := conn.ReadJSON(&msg); err != nil {
			mutex.Lock()
			delete(clients, conn)
			mutex.Unlock()
			broadcastPeers()
			return
		}

		switch msg.Type {

		case "register":
			mutex.Lock()

			if _, ok := users[msg.Username]; ok {
				mutex.Unlock()
				conn.WriteJSON(Packet{Type: "error"})
				continue
			}

			user := User{
				Username:  msg.Username,
				Password:  msg.Password,
				VirtualIP: randomIP(),
			}

			users[msg.Username] = user
			saveUsers()

			mutex.Unlock()

			conn.WriteJSON(Packet{
				Type:      "registered",
				VirtualIP: user.VirtualIP,
			})

		case "login":
			mutex.Lock()

			user, ok := users[msg.Username]

			if !ok || user.Password != msg.Password {
				mutex.Unlock()
				conn.WriteJSON(Packet{Type: "error"})
				continue
			}

			user.PublicAddr = msg.PublicAddr
			users[msg.Username] = user
			clients[conn] = user.Username

			saveUsers()
			mutex.Unlock()

			conn.WriteJSON(Packet{
				Type:       "login_success",
				VirtualIP:  user.VirtualIP,
				PublicAddr: user.PublicAddr,
			})

			broadcastPeers()
		}
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	loadUsers()

	http.HandleFunc("/ws", wsHandler)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
