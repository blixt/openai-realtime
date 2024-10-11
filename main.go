package main

import (
	"fmt"
	"log"
	"net"
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

	// Get all available IP addresses
	addrs := getAvailableAddresses(port)

	// Print server information
	fmt.Println("Server is accessible at the following addresses:")
	for _, addr := range addrs {
		fmt.Printf("  http://%s\n", addr)
	}

	// Listen on all interfaces
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// getAvailableAddresses returns a list of IP addresses the server is accessible on
func getAvailableAddresses(port string) []string {
	var addresses []string

	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Println("Error getting network interfaces:", err)
		return addresses
	}

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				// Skip loopback and link-local addresses
				if v.IP.IsLoopback() || v.IP.IsLinkLocalUnicast() {
					continue
				}
				if v.IP.To4() != nil {
					addresses = append(addresses, fmt.Sprintf("%s:%s", v.IP.String(), port))
				}
			}
		}
	}

	// Always include localhost
	addresses = append(addresses, fmt.Sprintf("localhost:%s", port))

	return addresses
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
		if err := room.HandleUserMessage(user, message); err != nil {
			log.Printf("Error handling user message: %v", err)
			user.WriteChan <- &chat.ErrorMessage{
				Error: "Failed to process message.",
			}
			continue
		}
	}
}
