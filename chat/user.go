package chat

import (
	"log"

	"github.com/blixt/openai-realtime/openai"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// User represents a connected user
type User struct {
	WS             *websocket.Conn
	WriteChan      chan Message
	PeerConnection *webrtc.PeerConnection
	OutputTrack    *webrtc.TrackLocalStaticSample
	OpenAIWriteCh  chan<- openai.Event
	done           chan struct{}
}

// NewUser creates a new User and starts its goroutines
func NewUser(ws *websocket.Conn) *User {
	user := &User{
		WS:        ws,
		WriteChan: make(chan Message, 256),
		done:      make(chan struct{}),
	}

	// Start the write goroutine
	go user.writeLoop()

	return user
}

// writeLoop handles writing messages to the WebSocket
func (u *User) writeLoop() {
	defer u.WS.Close()
	for {
		select {
		case msg := <-u.WriteChan:
			msgJSON, err := MarshalJSON(msg)
			if err != nil {
				log.Println("Error marshalling message:", err)
				return
			}
			err = u.WS.WriteMessage(websocket.TextMessage, msgJSON)
			if err != nil {
				log.Println("Error writing message:", err)
				return
			}
		case <-u.done:
			return
		}
	}
}

// Close gracefully shuts down the user's goroutines
func (u *User) Close() {
	close(u.done)
	close(u.WriteChan)
}
