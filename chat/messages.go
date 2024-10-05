package chat

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/blixt/openai-realtime/openai"
)

// Message is the interface that all messages implement.
type Message interface {
	GetType() string
	setType(string)
}

// BaseMessage represents the base structure of a WebSocket message
type BaseMessage struct {
	Type string `json:"type"`
}

// GetType returns the message type.
func (m *BaseMessage) GetType() string {
	return m.Type
}

// setType sets the message type.
func (m *BaseMessage) setType(t string) {
	m.Type = t
}

// WebRTCOfferMessage represents a WebRTC offer message
type WebRTCOfferMessage struct {
	BaseMessage
	SDP string `json:"sdp"`
}

// WebRTCAnswerMessage represents a WebRTC answer message
type WebRTCAnswerMessage struct {
	BaseMessage
	SDP string `json:"sdp"`
}

// WebRTCCandidateMessage represents an ICE candidate message
type WebRTCCandidateMessage struct {
	BaseMessage
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
	UsernameFragment *string `json:"usernameFragment"`
}

// MessageUpdate represents a message to/from the user
type MessageUpdate struct {
	BaseMessage
	MessageID string  `json:"message_id"`
	Content   *string `json:"content,omitempty"`
	Sender    *string `json:"sender,omitempty"` // "user" or "assistant"
	Audio     *bool   `json:"audio,omitempty"`
	IsDone    *bool   `json:"is_done,omitempty"`
}

// MessageDelta represents a delta update to a message
type MessageDelta struct {
	BaseMessage
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
	Audio     bool   `json:"audio"`
}

// FunctionAddMessage represents a request to add a function
type FunctionAddMessage struct {
	BaseMessage
	Schema openai.Tool `json:"schema"`
}

// FunctionRemoveMessage represents a request to remove a function
type FunctionRemoveMessage struct {
	BaseMessage
	Name string `json:"name"`
}

// FunctionExecuteMessage represents a function execution request
type FunctionExecuteMessage struct {
	BaseMessage
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FunctionResultMessage represents the result of a function execution
type FunctionResultMessage struct {
	BaseMessage
	Name   string  `json:"name"`
	Result *string `json:"result"`
}

// SessionUpdateMessage represents a session update message
type SessionUpdateMessage struct {
	BaseMessage
	Session *openai.SessionResource `json:"session"`
}

// TokenUpdateMessage represents a token usage update
type TokenUpdateMessage struct {
	BaseMessage
	Usage openai.Usage `json:"usage"`
}

// ErrorMessage represents an error message sent to the client
type ErrorMessage struct {
	BaseMessage
	Error string `json:"error"`
}

// messageTypes maps message types to their corresponding struct instances.
var messageTypes = map[string]Message{
	"webrtc.offer":     &WebRTCOfferMessage{},
	"webrtc.answer":    &WebRTCAnswerMessage{},
	"webrtc.candidate": &WebRTCCandidateMessage{},
	"message.update":   &MessageUpdate{},
	"message.delta":    &MessageDelta{},
	"function.add":     &FunctionAddMessage{},
	"function.remove":  &FunctionRemoveMessage{},
	"function.execute": &FunctionExecuteMessage{},
	"function.result":  &FunctionResultMessage{},
	"session.update":   &SessionUpdateMessage{},
	"token.update":     &TokenUpdateMessage{},
	"error":            &ErrorMessage{},
}

var (
	messageTypeToReflectType map[string]reflect.Type
	reflectTypeToMessageType map[reflect.Type]string
)

func init() {
	messageTypeToReflectType = make(map[string]reflect.Type)
	reflectTypeToMessageType = make(map[reflect.Type]string)

	for messageType, messageInstance := range messageTypes {
		reflectType := reflect.TypeOf(messageInstance).Elem()
		messageTypeToReflectType[messageType] = reflectType
		reflectTypeToMessageType[reflectType] = messageType
	}
}

// ParseMessage parses the JSON data and returns the appropriate Message struct.
func ParseMessage(data []byte) (Message, error) {
	// First, unmarshal into BaseMessage to get the Type
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("error unmarshalling BaseMessage: %w", err)
	}

	reflectType, ok := messageTypeToReflectType[base.Type]
	if !ok {
		return nil, fmt.Errorf("unknown message type: %q", base.Type)
	}

	// Create a new pointer to the appropriate struct
	message := reflect.New(reflectType).Interface().(Message)
	if err := json.Unmarshal(data, message); err != nil {
		return nil, fmt.Errorf("error unmarshalling message %s: %w", base.Type, err)
	}

	return message, nil
}

// MarshalJSON is a custom JSON marshaler for Message types.
func MarshalJSON(m Message) ([]byte, error) {
	reflectType := reflect.TypeOf(m).Elem()
	messageType, ok := reflectTypeToMessageType[reflectType]
	if !ok {
		return nil, fmt.Errorf("unknown message type: %T", m)
	}
	m.setType(messageType)
	return json.Marshal(m)
}
