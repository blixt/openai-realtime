package chat

import (
	"fmt"
	"log"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

// User represents a connected user
type User struct {
	WS             *websocket.Conn
	WriteChan      chan Message
	PeerConnection *webrtc.PeerConnection
	OutputTrack    *webrtc.TrackLocalStaticRTP
	Room           *Room
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
	if u.PeerConnection != nil {
		u.PeerConnection.Close()
	}
}

// RemoveTrack removes a track from the user's peer connection
func (user *User) RemoveTrack(track *webrtc.TrackLocalStaticRTP) error {
	if user.PeerConnection == nil {
		return nil // Nothing to do if there's no PeerConnection
	}
	var sender *webrtc.RTPSender
	for _, s := range user.PeerConnection.GetSenders() {
		if s.Track() == track {
			sender = s
			break
		}
	}
	if sender == nil {
		return fmt.Errorf("track not found")
	}
	return user.PeerConnection.RemoveTrack(sender)
}
