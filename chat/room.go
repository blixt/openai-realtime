package chat

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/blixt/openai-realtime/openai"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3/pkg/media"
)

// Add this utility function at the top of the file, after the imports

func ptr[T any](v T) *T {
	return &v
}

// Room represents a chat room
type Room struct {
	ID            string
	Users         map[*User]bool
	OpenAIConn    *websocket.Conn
	OpenAIWriteCh chan openai.Event
	Mu            sync.RWMutex
	Session       *openai.SessionResource
	Logger        *log.Logger
}

// NewRoom creates a new Room
func NewRoom(id string) *Room {
	// Create a log file for the room
	logFile, err := os.OpenFile(fmt.Sprintf("%s.log", id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to create log file for room %s: %v", id, err)
	}

	return &Room{
		ID:            id,
		Users:         make(map[*User]bool),
		OpenAIWriteCh: make(chan openai.Event, 256),
		Logger:        log.New(logFile, "", log.LstdFlags),
	}
}

// ConnectToOpenAI connects the room to OpenAI and starts handling messages
func (room *Room) ConnectToOpenAI() error {
	conn, err := ConnectToOpenAI()
	if err != nil {
		return err
	}
	room.OpenAIConn = conn

	// Start handling messages from OpenAI
	go room.HandleOpenAIMessages()

	// Start the OpenAI writer
	go room.writeLoop()

	return nil
}

// HandleOpenAIMessages handles messages from OpenAI for this room
func (room *Room) HandleOpenAIMessages() {
	for {
		_, message, err := room.OpenAIConn.ReadMessage()
		if err != nil {
			log.Println("Error reading from OpenAI:", err)
			room.OpenAIConn.Close()
			break
		}

		// Log the incoming message
		room.LogIncomingMessage(message)

		event, err := openai.ParseEvent(message)
		if err != nil {
			log.Println("Error parsing OpenAI event:", err)
			continue
		}

		// Handle events based on their type
		switch e := event.(type) {
		case *openai.SessionCreatedEvent:
			log.Printf("Session created: %s", e.Session.ID)

			// Send the initial session update
			sessionUpdateEvent := &openai.SessionUpdateEvent{
				Session: openai.SessionConfig{
					Instructions:      "Please assist the user.",
					Voice:             "alloy",
					InputAudioFormat:  "g711_ulaw",
					OutputAudioFormat: "g711_ulaw",
					// The OpenAI API doesn't allow this even if the docs says it's fine.
					// InputAudioTranscription: &openai.InputAudioTranscriptionConfig{
					// 	Enabled: true,
					// 	Model:   "whisper-1",
					// },
				},
			}
			room.OpenAIWriteCh <- sessionUpdateEvent

			// Update the room's session
			room.Mu.Lock()
			room.Session = &e.Session
			room.Mu.Unlock()

			// Broadcast the session update to all users
			msg := &SessionUpdateMessage{
				Session: &e.Session,
			}
			room.Broadcast(msg)

		case *openai.ConversationItemCreatedEvent:
			log.Printf("Conversation item created: ItemID: %s, Type: %s, Role: %s", e.Item.ID, e.Item.Type, e.Item.Role)
			msg := &MessageUpdate{
				MessageID: e.Item.ID,
				Content:   ptr(e.Item.String()),
				Sender:    ptr(e.Item.Role),
				Audio:     ptr(e.Item.Type == "audio"),
				IsDone:    ptr(e.Item.Status == "completed"),
			}
			room.Broadcast(msg)

		case *openai.ResponseTextDeltaEvent:
			msg := &MessageDelta{
				MessageID: e.ItemID,
				Content:   e.Delta,
				Audio:     false,
			}
			room.Broadcast(msg)

		case *openai.ResponseTextDoneEvent:
			msg := &MessageUpdate{
				MessageID: e.ItemID,
				Content:   ptr(e.Text),
				Sender:    ptr("assistant"),
				IsDone:    ptr(true),
			}
			room.Broadcast(msg)

		case *openai.ResponseAudioDeltaEvent:
			log.Printf("Audio delta received: ItemID: %s", e.ItemID)
			if e.Delta == "" {
				continue
			}
			// Note: The 'delta' is expected to be base64-encoded G.711 μ-law (PCMU) audio at 8kHz
			audioData, err := base64.StdEncoding.DecodeString(e.Delta)
			if err != nil {
				log.Println("Error decoding audio delta:", err)
				continue
			}

			room.Mu.RLock()
			for user := range room.Users {
				if user.OutputTrack != nil {
					err = user.OutputTrack.WriteSample(mediaSample(audioData))
					if err != nil {
						log.Println("Error sending audio to client:", err)
						continue
					}
				}
			}
			room.Mu.RUnlock()

		case *openai.ResponseAudioDoneEvent:
			log.Printf("Audio done: ItemID: %s", e.ItemID)
			// Consider sending a message to the UI indicating that audio playback is complete

		case *openai.ResponseFunctionCallArgumentsDoneEvent:
			log.Printf("Function execution requested: %s", e.CallID)
			msg := &FunctionExecuteMessage{
				Name:      e.Name,
				Arguments: e.Arguments,
			}
			room.Broadcast(msg)

		case *openai.ConversationItemInputAudioTranscriptionCompletedEvent:
			msg := &MessageUpdate{
				MessageID: e.ItemID,
				Content:   ptr(e.Transcript),
				Sender:    ptr("user"),
				Audio:     ptr(true),
				IsDone:    ptr(true),
			}
			room.Broadcast(msg)

		case *openai.SessionUpdatedEvent:
			session := e.Session

			room.Mu.Lock()
			room.Session = &session
			room.Mu.Unlock()

			msg := &SessionUpdateMessage{
				Session: &session,
			}
			room.Broadcast(msg)

		case *openai.ErrorEvent:
			log.Printf("Error from OpenAI: Type: %s, Code: %s, Message: %s",
				e.Error.Type, e.Error.Code, e.Error.Message)
			if e.Error.Param != nil {
				log.Printf("Error Param: %v", e.Error.Param)
			}
			if e.Error.EventID != nil {
				log.Printf("Error EventID: %v", e.Error.EventID)
			}

		case *openai.ConversationCreatedEvent:
			log.Printf("Conversation created: %s", e.Conversation.ID)

		case *openai.InputAudioBufferCommittedEvent:
			log.Printf("Input audio buffer committed: ItemID: %s", e.ItemID)

		case *openai.InputAudioBufferClearedEvent:
			log.Printf("Input audio buffer cleared")

		case *openai.InputAudioBufferSpeechStartedEvent:
			log.Printf("Speech started: ItemID: %s, AudioStartMs: %d", e.ItemID, e.AudioStartMs)

		case *openai.InputAudioBufferSpeechStoppedEvent:
			log.Printf("Speech stopped: ItemID: %s, AudioEndMs: %d", e.ItemID, e.AudioEndMs)

		case *openai.ConversationItemInputAudioTranscriptionFailedEvent:
			log.Printf("Input audio transcription failed: ItemID: %s, ContentIndex: %d, Error: %s", e.ItemID, e.ContentIndex, e.Error.Message)

		case *openai.ConversationItemTruncatedEvent:
			log.Printf("Conversation item truncated: ItemID: %s, ContentIndex: %d, AudioEndMs: %d", e.ItemID, e.ContentIndex, e.AudioEndMs)

		case *openai.ConversationItemDeletedEvent:
			log.Printf("Conversation item deleted: ItemID: %s", e.ItemID)

		case *openai.ResponseCreatedEvent:
			log.Printf("Response created: ResponseID: %s", e.Response.ID)

		case *openai.ResponseDoneEvent:
			log.Printf("Response done: ResponseID: %s, Status: %s", e.Response.ID, e.Response.Status)
			if e.Response.Usage != nil {
				tokenUpdate := &TokenUpdateMessage{
					Usage: *e.Response.Usage,
				}
				room.Broadcast(tokenUpdate)
			}

		case *openai.ResponseOutputItemAddedEvent:
			log.Printf("Response output item added: ResponseID: %s, OutputIndex: %d, ItemID: %s", e.ResponseID, e.OutputIndex, e.Item.ID)

		case *openai.ResponseOutputItemDoneEvent:
			log.Printf("Response output item done: ResponseID: %s, OutputIndex: %d, ItemID: %s", e.ResponseID, e.OutputIndex, e.Item.ID)

		case *openai.ResponseContentPartAddedEvent:
			log.Printf("Response content part added: ResponseID: %s, ItemID: %s, OutputIndex: %d, ContentIndex: %d", e.ResponseID, e.ItemID, e.OutputIndex, e.ContentIndex)

		case *openai.ResponseContentPartDoneEvent:
			log.Printf("Response content part done: ResponseID: %s, ItemID: %s, OutputIndex: %d, ContentIndex: %d", e.ResponseID, e.ItemID, e.OutputIndex, e.ContentIndex)

		case *openai.ResponseAudioTranscriptDeltaEvent:
			log.Printf("Audio transcript delta: ResponseID: %s, ItemID: %s, OutputIndex: %d, ContentIndex: %d, Delta: %s", e.ResponseID, e.ItemID, e.OutputIndex, e.ContentIndex, e.Delta)
			msg := &MessageDelta{
				MessageID: e.ItemID,
				Content:   e.Delta,
				Audio:     true,
			}
			room.Broadcast(msg)

		case *openai.ResponseAudioTranscriptDoneEvent:
			log.Printf("Audio transcript done: ResponseID: %s, ItemID: %s, OutputIndex: %d, ContentIndex: %d, Transcript: %s", e.ResponseID, e.ItemID, e.OutputIndex, e.ContentIndex, e.Transcript)
			msg := &MessageUpdate{
				MessageID: e.ItemID,
				Content:   ptr(e.Transcript),
				Sender:    ptr("assistant"),
				Audio:     ptr(true),
				IsDone:    ptr(true),
			}
			room.Broadcast(msg)

		case *openai.ResponseFunctionCallArgumentsDeltaEvent:
			log.Printf("Function call arguments delta: CallID: %s, Delta: %s", e.CallID, e.Delta)
			// You might want to accumulate these deltas or send them to the UI

		case *openai.RateLimitsUpdatedEvent:
			log.Printf("Rate limits updated: %+v", e.RateLimits)
			// You might want to store or act on these rate limits

		default:
			log.Printf("Unhandled event type from OpenAI: %T", e)
		}
	}

	// The OpenAI connection is closed, so we can clean up
	room.Close()
}

// AddUser adds a user to the room
func (room *Room) AddUser(user *User) {
	room.Mu.Lock()
	defer room.Mu.Unlock()
	room.Users[user] = true
	user.OpenAIWriteCh = room.OpenAIWriteCh
	if room.Session != nil {
		user.WriteChan <- &SessionUpdateMessage{
			Session: room.Session,
		}
	}
}

// RemoveUser removes a user from the room
func (room *Room) RemoveUser(user *User) {
	room.Mu.Lock()
	defer room.Mu.Unlock()
	delete(room.Users, user)
}

// Broadcast sends a message to all users in the room
func (room *Room) Broadcast(msg Message) {
	room.Mu.RLock()
	defer room.Mu.RUnlock()
	// FIXME: The message is a pointer which will be shared across the different
	// users and the setType method modifies the message which causes a data
	// race.
	for user := range room.Users {
		select {
		case user.WriteChan <- msg:
		default:
			log.Println("Failed to send message to user because write channel is full")
		}
	}
}

// Close closes the room and all associated resources
func (room *Room) Close() {
	room.Mu.Lock()
	defer room.Mu.Unlock()
	if room.OpenAIConn != nil {
		room.OpenAIConn.Close()
	}
	for user := range room.Users {
		user.Close()
	}
}

// HandleUserMessage processes incoming messages from users
func (room *Room) HandleUserMessage(user *User, message []byte) {
	// Parse the incoming message
	msg, err := ParseMessage(message)
	if err != nil {
		log.Printf("Error parsing %q: %s", message, err)
		return
	}

	switch m := msg.(type) {
	case *WebRTCOfferMessage:
		err = user.SetupWebRTC(m.SDP)
		if err != nil {
			log.Println("Error setting up WebRTC:", err)
			return
		}

	case *WebRTCCandidateMessage:
		err = user.HandleICECandidate(m.Candidate, m.SDPMid, m.SDPMLineIndex, m.UsernameFragment)
		if err != nil {
			log.Println("Error handling ICE candidate:", err)
			return
		}

	case *MessageUpdate:
		// Forward message from the user to OpenAI
		event := &openai.ConversationItemCreateEvent{
			Item: openai.Item{
				ID:      m.MessageID,
				Type:    "message",
				Role:    "user",
				Content: []openai.ContentPart{{Type: "input_text", Text: m.Content}},
				Status:  "completed",
			},
		}
		user.OpenAIWriteCh <- event
		room.Broadcast(&MessageUpdate{
			MessageID: m.MessageID,
			Content:   m.Content,
			Sender:    ptr("user"),
			IsDone:    ptr(true),
		})

	case *FunctionAddMessage:
		room.Mu.Lock()
		if room.Session == nil {
			room.Mu.Unlock()
			log.Println("Session not initialized")
			return
		}

		// Add the new tool directly from the message
		room.Session.Tools = append(room.Session.Tools, m.Schema)

		// Send the updated session to OpenAI
		updateEvent := &openai.SessionUpdateEvent{
			Session: openai.SessionConfig{
				Tools: room.Session.Tools,
			},
		}
		user.OpenAIWriteCh <- updateEvent
		room.Mu.Unlock()

	case *FunctionRemoveMessage:
		room.Mu.Lock()
		if room.Session == nil {
			room.Mu.Unlock()
			log.Println("Session not initialized")
			return
		}

		for i, tool := range room.Session.Tools {
			if tool.Name == m.Name {
				room.Session.Tools = append(room.Session.Tools[:i], room.Session.Tools[i+1:]...)
				break
			}
		}

		// Send the updated session to OpenAI
		updateEvent := &openai.SessionUpdateEvent{
			Session: openai.SessionConfig{
				Tools: room.Session.Tools,
			},
		}
		user.OpenAIWriteCh <- updateEvent
		room.Mu.Unlock()

	case *FunctionResultMessage:
		// Forward function result from the user to OpenAI
		event := &openai.ConversationItemCreateEvent{
			Item: openai.Item{
				Type: "function_call_output",
				Role: "user",
				Content: []openai.ContentPart{
					{Type: "text", Text: m.Result},
				},
			},
		}
		user.OpenAIWriteCh <- event

	default:
		log.Printf("Unknown message type: %T", m)
	}
}

// writeLoop handles writing messages to the WebSocket
func (room *Room) writeLoop() {
	for event := range room.OpenAIWriteCh {
		eventJSON, err := openai.MarshalJSON(event)
		if err != nil {
			log.Println("Error marshalling event:", err)
			continue
		}
		// Log the outgoing message
		room.Logger.Printf("> %s", string(eventJSON))
		err = room.OpenAIConn.WriteMessage(websocket.TextMessage, eventJSON)
		if err != nil {
			log.Println("Error writing to OpenAI:", err)
		}
	}
}

// LogIncomingMessage logs incoming messages from OpenAI
func (room *Room) LogIncomingMessage(data []byte) {
	room.Logger.Printf("< %s", string(data))
}

// Create a media sample from audio data
// Note: The 'data' parameter must be G.711 μ-law (PCMU) encoded audio at 8kHz
func mediaSample(data []byte) media.Sample {
	return media.Sample{
		Data:     data,
		Duration: time.Millisecond * 20, // 20ms packets
	}
}
