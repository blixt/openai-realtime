package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/blixt/openai-realtime/chat"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// WebSocket upgrader
var upgrader = websocket.Upgrader{}

// Global rooms map and mutex
var (
	rooms   = make(map[string]*chat.Room)
	roomsMu sync.RWMutex
)

func init() {
	godotenv.Overload(".env.local")
}

func main() {
	// Serve static files
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/", fs)

	// WebSocket endpoint for signaling and messaging
	http.HandleFunc("/ws", handleWebSocket)

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server listening on http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Handle WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a WebSocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	// Create a new user
	user := chat.NewUser(ws)
	defer user.Close()

	// For now, we hardcode the room ID
	roomID := "abcdef"

	// Get or create the room
	roomsMu.Lock()
	room, ok := rooms[roomID]
	if !ok {
		// Create a new room
		room = chat.NewRoom(roomID)
		rooms[roomID] = room

		// Connect to OpenAI Realtime API
		if err := room.ConnectToOpenAI(); err != nil {
			log.Println("Failed to connect to OpenAI Realtime API:", err)
			user.WriteChan <- &chat.ErrorMessage{
				Error: "Failed to connect to OpenAI.",
			}
			roomsMu.Unlock()
			return
		}
	}
	roomsMu.Unlock()

	// Add user to the room
	room.AddUser(user)
	defer room.RemoveUser(user)

	// Handle incoming WebSocket messages from the user
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println("WebSocket Read error:", err)
			break
		}

		// Process the incoming message
		room.HandleUserMessage(user, message)
	}
}
