package main

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Be cautious with this in production
	},
}

var clients map[*websocket.Conn]bool = make(map[*websocket.Conn]bool)
var peerMap map[*websocket.Conn]*websocket.Conn = make(map[*websocket.Conn]*websocket.Conn)
var mutex sync.Mutex

func addClient(conn *websocket.Conn) {
	defer mutex.Unlock()
	mutex.Lock()
	clients[conn] = true
}

func removeClient(conn *websocket.Conn) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(clients, conn)
	if peer, ok := peerMap[conn]; ok {
		delete(peerMap, peer)
	}
	delete(peerMap, conn)
}
func getRandomPeer(currentClient *websocket.Conn) *websocket.Conn {
	mutex.Lock()
	defer mutex.Unlock()
	var availablePeers = []*websocket.Conn{}

	for client := range clients {
		if currentClient != client && peerMap[client] == nil {
			availablePeers = append(availablePeers, client)
		}
	}
	if len(availablePeers) == 0 {
		return nil
	}
	peer := availablePeers[rand.IntN(len(availablePeers))]
	peerMap[currentClient] = peer
	peerMap[peer] = currentClient
	return peer

}

func SendMessage(src *websocket.Conn, dest *websocket.Conn) {
	for {
		messageType, message, err := src.ReadMessage()
		if err != nil {
			fmt.Println(err)
			return
		}
		if err := dest.WriteMessage(messageType, message); err != nil {
			fmt.Println(err)
			return
		}
	}
}

func Connector(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()
	addClient(conn)
	defer removeClient(conn)

	peerFound := make(chan struct{})
	go func() {
		for {
			mutex.Lock()
			peer := peerMap[conn]
			mutex.Unlock()
			if peer != nil {
				close(peerFound)
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {

		case <-ticker.C:
			peer := getRandomPeer(conn)
			if peer == nil {
				// Send periodic update
				conn.WriteMessage(websocket.TextMessage, []byte("Still waiting for a peer..."))
			}
		case <-peerFound:
			goto PeerFound

		case <-timeout:
			conn.WriteMessage(websocket.TextMessage, []byte("No peer found. Please try again later."))
			return
		}
	}

PeerFound:
	mutex.Lock()
	peer := peerMap[conn]
	mutex.Unlock()
	if peer != nil {
		go SendMessage(conn, peer)
		SendMessage(peer, conn)
	}
	/*
		Connector function
		│
		├─► New Goroutine: handleMessages(conn, peer)
		│   │
		│   └─► Continuously read from conn and send to peer
		│
		└─► Main Thread: handleMessages(peer, conn)
		    │
		    └─► Continuously read from peer and send to conn
	*/

}

func main() {
	r := gin.Default()
	r.GET("/connect", func(c *gin.Context) {
		fmt.Println("here")
		Connector(c.Writer, c.Request)
	})
	r.Run()

}
