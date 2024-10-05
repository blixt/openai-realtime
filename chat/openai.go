package chat

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectToOpenAI connects to the OpenAI Realtime API
func ConnectToOpenAI() (*websocket.Conn, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}

	openAIConn, resp, err := websocket.DefaultDialer.Dial(
		"wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01",
		http.Header{
			"Authorization": []string{"Bearer " + apiKey},
			"OpenAI-Beta":   []string{"realtime=v1"},
		},
	)
	if err != nil {
		if resp != nil {
			bodyBytes, readErr := io.ReadAll(resp.Body)
			bodyString := "unable to read response body"
			if readErr == nil {
				if len(bodyBytes) == 0 {
					bodyString = "(empty)"
				} else {
					bodyString = fmt.Sprintf("%q", bodyBytes)
				}
			}
			resp.Body.Close()
			return nil, fmt.Errorf("request failed: %w (%s, body: %s)", err, resp.Status, bodyString)
		}
		return nil, err
	}

	// Add a ping handler to detect disconnection
	openAIConn.SetPingHandler(func(appData string) error {
		err := openAIConn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
		if err != nil {
			log.Println("OpenAI connection lost:", err)
		}
		return nil
	})

	return openAIConn, nil
}
