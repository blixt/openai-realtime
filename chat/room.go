package chat

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/blixt/openai-realtime/openai"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
)

func ptr[T any](v T) *T {
	return &v
}

type Room struct {
	ID               string
	users            map[*User]bool
	openAIConn       *websocket.Conn
	openAIWriteCh    chan openai.Event
	mu               sync.RWMutex
	session          *openai.SessionResource
	logger           *log.Logger
	audioTracks      map[*User]*webrtc.TrackLocalStaticRTP
	sampleAudioTrack *webrtc.TrackLocalStaticSample
	tracksMu         sync.RWMutex
	audioQueue       [][]byte
	audioQueueMu     sync.Mutex
}

// NewRoom creates a new Room
func NewRoom(id string) *Room {
	// Create a log file for the room
	logFile, err := os.OpenFile(fmt.Sprintf("%s.log", id), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to create log file for room %s: %v", id, err)
	}
	room := &Room{
		ID:            id,
		users:         make(map[*User]bool),
		openAIWriteCh: make(chan openai.Event, 256),
		logger:        log.New(logFile, "", log.LstdFlags),
		audioTracks:   make(map[*User]*webrtc.TrackLocalStaticRTP),
	}

	// Create the sample audio track
	sampleTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, "sampleaudio", "pion")
	if err != nil {
		log.Printf("Error creating sample audio track: %v", err)
	} else {
		room.sampleAudioTrack = sampleTrack
	}

	go room.streamSampleAudio()
	return room
}

// ConnectToOpenAI connects the room to OpenAI and starts handling messages
func (room *Room) ConnectToOpenAI() error {
	conn, err := ConnectToOpenAI()
	if err != nil {
		return err
	}
	room.openAIConn = conn

	// Start handling messages from OpenAI
	go room.handleOpenAIMessages()

	// Start the OpenAI writer
	go room.writeLoop()

	return nil
}

// handleOpenAIMessages handles messages from OpenAI for this room
func (room *Room) handleOpenAIMessages() {
	for {
		_, message, err := room.openAIConn.ReadMessage()
		if err != nil {
			log.Println("Error reading from OpenAI:", err)
			room.openAIConn.Close()
			break
		}

		// Log the incoming message
		room.logIncomingMessage(message)

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
			room.openAIWriteCh <- sessionUpdateEvent

			// Update the room's session
			room.mu.Lock()
			room.session = &e.Session
			room.mu.Unlock()

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
			log.Printf("Audio delta received: ItemID: %s, Delta length: %d", e.ItemID, len(e.Delta))
			if e.Delta == "" {
				continue
			}
			// Note: The 'delta' is expected to be base64-encoded G.711 μ-law (PCMU) audio at 8kHz
			audioData, err := base64.StdEncoding.DecodeString(e.Delta)
			if err != nil {
				log.Println("Error decoding audio delta:", err)
				continue
			}
			// Add the audio data to the queue
			room.addAudioChunk(audioData)

			// TODO: Implement a separate goroutine to process the audio queue
			// TODO: Convert the audio data to the appropriate format for WebRTC
			// TODO: Split the audio data into smaller chunks suitable for RTP packets
			// TODO: Send the audio data over time to match real-time playback

		case *openai.ResponseAudioDoneEvent:
			log.Printf("Audio done: ItemID: %s", e.ItemID)

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

			room.mu.Lock()
			room.session = &session
			room.mu.Unlock()

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
	room.mu.Lock()
	defer room.mu.Unlock()
	room.users[user] = true
	user.Room = room
	if room.session != nil {
		user.WriteChan <- &SessionUpdateMessage{
			Session: room.session,
		}
	}
}

// RemoveUser removes a user from the room
func (room *Room) RemoveUser(user *User) {
	room.mu.Lock()
	defer room.mu.Unlock()
	delete(room.users, user)

	room.tracksMu.Lock()
	userTrack := room.audioTracks[user]
	delete(room.audioTracks, user)
	room.tracksMu.Unlock()

	// Remove the user's track from all other users
	for otherUser := range room.users {
		if otherUser == user {
			continue
		}
		if err := otherUser.RemoveTrack(userTrack); err != nil {
			log.Printf("Error removing track from user %p: %v", otherUser, err)
		}
	}

	// Remove all other users' tracks from the leaving user
	room.tracksMu.RLock()
	for _, track := range room.audioTracks {
		if err := user.RemoveTrack(track); err != nil {
			log.Printf("Error removing track from leaving user %p: %v", user, err)
		}
	}
	room.tracksMu.RUnlock()

	if user.PeerConnection != nil {
		if err := user.PeerConnection.Close(); err != nil {
			log.Printf("Error closing PeerConnection for user %p: %v", user, err)
		}
	}
}

// Broadcast sends a message to all users in the room
func (room *Room) Broadcast(msg Message) {
	room.mu.RLock()
	defer room.mu.RUnlock()
	// FIXME: The message is a pointer which will be shared across the different
	// users and the setType method modifies the message which causes a data
	// race.
	for user := range room.users {
		select {
		case user.WriteChan <- msg:
		default:
			log.Println("Failed to send message to user because write channel is full")
		}
	}
}

// Close closes the room and all associated resources
func (room *Room) Close() {
	room.mu.Lock()
	defer room.mu.Unlock()
	if room.openAIConn != nil {
		room.openAIConn.Close()
	}
	for user := range room.users {
		user.Close()
	}
}

// HandleUserMessage processes incoming messages from users
func (room *Room) HandleUserMessage(user *User, message []byte) error {
	// Parse the incoming message
	msg, err := ParseMessage(message)
	if err != nil {
		return fmt.Errorf("error parsing message: %w", err)
	}

	switch m := msg.(type) {
	case *WebRTCOfferMessage:
		if err := user.SetupWebRTC(m.SDP); err != nil {
			return fmt.Errorf("error setting up WebRTC: %w", err)
		}

	case *WebRTCCandidateMessage:
		err = user.HandleICECandidate(m.Candidate, m.SDPMid, m.SDPMLineIndex, m.UsernameFragment)
		if err != nil {
			log.Println("Error handling ICE candidate:", err)
			return err
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
		user.Room.openAIWriteCh <- event
		room.Broadcast(&MessageUpdate{
			MessageID: m.MessageID,
			Content:   m.Content,
			Sender:    ptr("user"),
			IsDone:    ptr(true),
		})

	case *FunctionAddMessage:
		room.mu.Lock()
		if room.session == nil {
			room.mu.Unlock()
			log.Println("Session not initialized")
			return fmt.Errorf("session not initialized")
		}

		// Add the new tool directly from the message
		room.session.Tools = append(room.session.Tools, m.Schema)

		// Send the updated session to OpenAI
		updateEvent := &openai.SessionUpdateEvent{
			Session: openai.SessionConfig{
				Tools: room.session.Tools,
			},
		}
		user.Room.openAIWriteCh <- updateEvent
		room.mu.Unlock()

	case *FunctionRemoveMessage:
		room.mu.Lock()
		if room.session == nil {
			room.mu.Unlock()
			log.Println("Session not initialized")
			return fmt.Errorf("session not initialized")
		}

		for i, tool := range room.session.Tools {
			if tool.Name == m.Name {
				room.session.Tools = append(room.session.Tools[:i], room.session.Tools[i+1:]...)
				break
			}
		}

		// Send the updated session to OpenAI
		updateEvent := &openai.SessionUpdateEvent{
			Session: openai.SessionConfig{
				Tools: room.session.Tools,
			},
		}
		user.Room.openAIWriteCh <- updateEvent
		room.mu.Unlock()

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
		user.Room.openAIWriteCh <- event

	default:
		return fmt.Errorf("unknown message type: %T", m)
	}

	return nil
}

// writeLoop handles writing messages to the WebSocket
func (room *Room) writeLoop() {
	for event := range room.openAIWriteCh {
		eventJSON, err := openai.MarshalJSON(event)
		if err != nil {
			log.Println("Error marshalling event:", err)
			continue
		}
		// Log the outgoing message
		room.logger.Printf("> %s", string(eventJSON))
		err = room.openAIConn.WriteMessage(websocket.TextMessage, eventJSON)
		if err != nil {
			log.Println("Error writing to OpenAI:", err)
		}
	}
}

// logIncomingMessage logs incoming messages from OpenAI
func (room *Room) logIncomingMessage(data []byte) {
	room.logger.Printf("< %s", string(data))
}

// broadcastAudio sends audio data to all users in the room except the sender
func (room *Room) broadcastAudio(sender *User, packet *rtp.Packet) {
	room.tracksMu.RLock()
	defer room.tracksMu.RUnlock()

	for user, track := range room.audioTracks {
		if user == sender {
			continue
		}
		err := track.WriteRTP(packet)
		if err != nil {
			log.Printf("Error broadcasting audio to user %p: %v", user, err)
		}
	}
}

// streamSampleAudio streams sample audio to all users in the room
func (room *Room) streamSampleAudio() {
	if room.sampleAudioTrack == nil {
		log.Println("Sample audio track not initialized")
		return
	}

	audioFileName := "audio.ogg"
	file, err := os.Open(audioFileName)
	if err != nil {
		log.Printf("Error opening audio file: %v", err)
		return
	}
	defer file.Close()

	ogg, _, err := oggreader.NewWith(file)
	if err != nil {
		log.Printf("Error creating OGG reader: %v", err)
		return
	}

	var lastGranule uint64
	oggPageDuration := time.Millisecond * 20
	ticker := time.NewTicker(oggPageDuration)
	defer ticker.Stop()

	for range ticker.C {
		pageData, pageHeader, err := ogg.ParseNextPage()
		if err != nil {
			if err == io.EOF {
				// Restart the audio file
				file.Seek(0, 0)
				ogg, _, _ = oggreader.NewWith(file)
				continue
			}
			log.Printf("Error parsing OGG page: %v", err)
			return
		}

		sampleCount := float64(pageHeader.GranulePosition - lastGranule)
		lastGranule = pageHeader.GranulePosition
		sampleDuration := time.Duration((sampleCount/48000)*1000) * time.Millisecond

		if err := room.sampleAudioTrack.WriteSample(media.Sample{Data: pageData, Duration: sampleDuration}); err != nil {
			log.Printf("Error writing audio sample: %v", err)
			return
		}
	}
}

// addAudioChunk adds an audio chunk to the queue
func (room *Room) addAudioChunk(data []byte) {
	room.audioQueueMu.Lock()
	defer room.audioQueueMu.Unlock()
	room.audioQueue = append(room.audioQueue, data)
}

// TODO: Implement a method to process the audio queue
// This method should run in a separate goroutine
// It should:
// 1. Take audio chunks from the queue
// 2. Convert them to the appropriate format for WebRTC (if necessary)
// 3. Split them into smaller chunks suitable for RTP packets
// 4. Send them over time to match real-time playback

// setupUserWebRTC sets up WebRTC for a user
func (room *Room) setupUserWebRTC(user *User) error {
	if user.PeerConnection == nil {
		return fmt.Errorf("user PeerConnection is nil")
	}

	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  1,
		},
		"audio",
		"pion",
	)
	if err != nil {
		return fmt.Errorf("error creating audio track: %w", err)
	}

	room.tracksMu.Lock()
	room.audioTracks[user] = audioTrack
	room.tracksMu.Unlock()

	// Add the new user's track to all existing users
	room.mu.RLock()
	for existingUser := range room.users {
		if existingUser != user && existingUser.PeerConnection != nil {
			if _, err := existingUser.PeerConnection.AddTrack(audioTrack); err != nil {
				log.Printf("Error adding track to existing user %p: %v", existingUser, err)
			}
		}
	}
	room.mu.RUnlock()

	// Add all existing users' tracks to the new user
	room.tracksMu.RLock()
	for existingUser, existingTrack := range room.audioTracks {
		if existingUser != user {
			if _, err := user.PeerConnection.AddTrack(existingTrack); err != nil {
				log.Printf("Error adding existing track to new user %p: %v", user, err)
			}
		}
	}
	room.tracksMu.RUnlock()

	return nil
}
