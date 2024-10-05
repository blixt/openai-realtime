package openai

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Event is the interface that all events implement.
type Event interface {
	GetType() string
	setType(string)
}

// BaseEvent contains common fields for all events.
type BaseEvent struct {
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
}

// GetType returns the event type.
func (e *BaseEvent) GetType() string {
	return e.Type
}

// setType sets the event type.
func (e *BaseEvent) setType(t string) {
	e.Type = t
}

// -----------------------------------
// Client Events
// -----------------------------------

// SessionUpdateEvent represents a session.update event sent from client to server.
type SessionUpdateEvent struct {
	BaseEvent
	Session SessionConfig `json:"session"`
}

// InputAudioBufferAppendEvent represents input_audio_buffer.append event sent from client to server.
type InputAudioBufferAppendEvent struct {
	BaseEvent
	Audio string `json:"audio"`
}

// InputAudioBufferCommitEvent represents input_audio_buffer.commit event sent from client to server.
type InputAudioBufferCommitEvent struct {
	BaseEvent
}

// InputAudioBufferClearEvent represents input_audio_buffer.clear event sent from client to server.
type InputAudioBufferClearEvent struct {
	BaseEvent
}

// ConversationItemCreateEvent represents conversation.item.create event sent from client to server.
type ConversationItemCreateEvent struct {
	BaseEvent
	PreviousItemID *string `json:"previous_item_id,omitempty"`
	Item           Item    `json:"item"`
}

// ConversationItemTruncateEvent represents conversation.item.truncate event sent from client to server.
type ConversationItemTruncateEvent struct {
	BaseEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	AudioEndMs   int    `json:"audio_end_ms"`
}

// ConversationItemDeleteEvent represents conversation.item.delete event sent from client to server.
type ConversationItemDeleteEvent struct {
	BaseEvent
	ItemID string `json:"item_id"`
}

// ResponseCreateEvent represents response.create event sent from client to server.
type ResponseCreateEvent struct {
	BaseEvent
	Response ResponseConfig `json:"response"`
}

// ResponseCancelEvent represents response.cancel event sent from client to server.
type ResponseCancelEvent struct {
	BaseEvent
}

// -----------------------------------
// Server Events
// -----------------------------------

// ErrorEvent represents an error event from the server.
type ErrorEvent struct {
	BaseEvent
	Error ErrorDetail `json:"error"`
}

// SessionCreatedEvent represents session.created event from the server.
type SessionCreatedEvent struct {
	BaseEvent
	Session SessionResource `json:"session"`
}

// SessionUpdatedEvent represents session.updated event from the server.
type SessionUpdatedEvent struct {
	BaseEvent
	Session SessionResource `json:"session"`
}

// ConversationCreatedEvent represents conversation.created event from the server.
type ConversationCreatedEvent struct {
	BaseEvent
	Conversation Conversation `json:"conversation"`
}

// InputAudioBufferCommittedEvent represents input_audio_buffer.committed event from the server.
type InputAudioBufferCommittedEvent struct {
	BaseEvent
	PreviousItemID *string `json:"previous_item_id,omitempty"`
	ItemID         string  `json:"item_id"`
}

// InputAudioBufferClearedEvent represents input_audio_buffer.cleared event from the server.
type InputAudioBufferClearedEvent struct {
	BaseEvent
}

// InputAudioBufferSpeechStartedEvent represents input_audio_buffer.speech_started event from the server.
type InputAudioBufferSpeechStartedEvent struct {
	BaseEvent
	AudioStartMs int    `json:"audio_start_ms"`
	ItemID       string `json:"item_id"`
}

// InputAudioBufferSpeechStoppedEvent represents input_audio_buffer.speech_stopped event from the server.
type InputAudioBufferSpeechStoppedEvent struct {
	BaseEvent
	AudioEndMs int    `json:"audio_end_ms"`
	ItemID     string `json:"item_id"`
}

// ConversationItemCreatedEvent represents conversation.item.created event from the server.
type ConversationItemCreatedEvent struct {
	BaseEvent
	PreviousItemID *string `json:"previous_item_id,omitempty"`
	Item           Item    `json:"item"`
}

// ConversationItemInputAudioTranscriptionCompletedEvent represents conversation.item.input_audio_transcription.completed event from the server.
type ConversationItemInputAudioTranscriptionCompletedEvent struct {
	BaseEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

// ConversationItemInputAudioTranscriptionFailedEvent represents conversation.item.input_audio_transcription.failed event from the server.
type ConversationItemInputAudioTranscriptionFailedEvent struct {
	BaseEvent
	ItemID       string      `json:"item_id"`
	ContentIndex int         `json:"content_index"`
	Error        ErrorDetail `json:"error"`
}

// ConversationItemTruncatedEvent represents conversation.item.truncated event from the server.
type ConversationItemTruncatedEvent struct {
	BaseEvent
	ItemID       string `json:"item_id"`
	ContentIndex int    `json:"content_index"`
	AudioEndMs   int    `json:"audio_end_ms"`
}

// ConversationItemDeletedEvent represents conversation.item.deleted event from the server.
type ConversationItemDeletedEvent struct {
	BaseEvent
	ItemID string `json:"item_id"`
}

// ResponseCreatedEvent represents response.created event from the server.
type ResponseCreatedEvent struct {
	BaseEvent
	Response ResponseResource `json:"response"`
}

// ResponseDoneEvent represents response.done event from the server.
type ResponseDoneEvent struct {
	BaseEvent
	Response ResponseResource `json:"response"`
}

// ResponseOutputItemAddedEvent represents response.output_item.added event from the server.
type ResponseOutputItemAddedEvent struct {
	BaseEvent
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Item        Item   `json:"item"`
}

// ResponseOutputItemDoneEvent represents response.output_item.done event from the server.
type ResponseOutputItemDoneEvent struct {
	BaseEvent
	ResponseID  string `json:"response_id"`
	OutputIndex int    `json:"output_index"`
	Item        Item   `json:"item"`
}

// ResponseContentPartAddedEvent represents response.content_part.added event from the server.
type ResponseContentPartAddedEvent struct {
	BaseEvent
	ResponseID   string      `json:"response_id"`
	ItemID       string      `json:"item_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Part         ContentPart `json:"part"`
}

// ResponseContentPartDoneEvent represents response.content_part.done event from the server.
type ResponseContentPartDoneEvent struct {
	BaseEvent
	ResponseID   string      `json:"response_id"`
	ItemID       string      `json:"item_id"`
	OutputIndex  int         `json:"output_index"`
	ContentIndex int         `json:"content_index"`
	Part         ContentPart `json:"part"`
}

// ResponseTextDeltaEvent represents response.text.delta event from the server.
type ResponseTextDeltaEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ResponseTextDoneEvent represents response.text.done event from the server.
type ResponseTextDoneEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Text         string `json:"text"`
}

// ResponseAudioTranscriptDeltaEvent represents response.audio_transcript.delta event from the server.
type ResponseAudioTranscriptDeltaEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ResponseAudioTranscriptDoneEvent represents response.audio_transcript.done event from the server.
type ResponseAudioTranscriptDoneEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Transcript   string `json:"transcript"`
}

// ResponseAudioDeltaEvent represents response.audio.delta event from the server.
type ResponseAudioDeltaEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
	Delta        string `json:"delta"`
}

// ResponseAudioDoneEvent represents response.audio.done event from the server.
type ResponseAudioDoneEvent struct {
	BaseEvent
	ResponseID   string `json:"response_id"`
	ItemID       string `json:"item_id"`
	OutputIndex  int    `json:"output_index"`
	ContentIndex int    `json:"content_index"`
}

// ResponseFunctionCallArgumentsDeltaEvent represents response.function_call_arguments.delta event from the server.
type ResponseFunctionCallArgumentsDeltaEvent struct {
	BaseEvent
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Delta       string `json:"delta"`
}

// ResponseFunctionCallArgumentsDoneEvent represents response.function_call_arguments.done event from the server.
type ResponseFunctionCallArgumentsDoneEvent struct {
	BaseEvent
	ResponseID  string `json:"response_id"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	CallID      string `json:"call_id"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
}

// RateLimitsUpdatedEvent represents rate_limits.updated event from the server.
type RateLimitsUpdatedEvent struct {
	BaseEvent
	RateLimits []RateLimit `json:"rate_limits"`
}

// -----------------------------------
// Common Structs
// -----------------------------------

// ErrorDetail represents details of an error.
type ErrorDetail struct {
	Type    string      `json:"type"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Param   interface{} `json:"param"`
	EventID interface{} `json:"event_id,omitempty"`
}

// SessionConfig represents the session configuration sent from the client.
type SessionConfig struct {
	Modalities              []string                       `json:"modalities,omitempty"`
	Instructions            string                         `json:"instructions,omitempty"`
	Voice                   string                         `json:"voice,omitempty"`
	InputAudioFormat        string                         `json:"input_audio_format,omitempty"`
	OutputAudioFormat       string                         `json:"output_audio_format,omitempty"`
	InputAudioTranscription *InputAudioTranscriptionConfig `json:"input_audio_transcription,omitempty"`
	TurnDetection           *TurnDetectionConfig           `json:"turn_detection,omitempty"`
	Tools                   []Tool                         `json:"tools,omitempty"`
	ToolChoice              string                         `json:"tool_choice,omitempty"`
	Temperature             float64                        `json:"temperature,omitempty"`
	MaxOutputTokens         interface{}                    `json:"max_output_tokens,omitempty"` // Can be null or "inf"
}

// SessionResource represents the session resource returned by the server.
type SessionResource struct {
	ExpiresAt               *int64                         `json:"expires_at,omitempty"`
	ID                      string                         `json:"id"`
	Object                  string                         `json:"object"`
	Model                   string                         `json:"model"`
	Modalities              []string                       `json:"modalities"`
	Instructions            string                         `json:"instructions"`
	Voice                   string                         `json:"voice"`
	InputAudioFormat        string                         `json:"input_audio_format"`
	OutputAudioFormat       string                         `json:"output_audio_format"`
	InputAudioTranscription *InputAudioTranscriptionConfig `json:"input_audio_transcription,omitempty"`
	TurnDetection           *TurnDetectionConfig           `json:"turn_detection,omitempty"`
	Tools                   []Tool                         `json:"tools"`
	ToolChoice              string                         `json:"tool_choice"`
	Temperature             float64                        `json:"temperature"`
	MaxOutputTokens         interface{}                    `json:"max_output_tokens"`
}

// InputAudioTranscriptionConfig represents input audio transcription settings.
type InputAudioTranscriptionConfig struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
}

// TurnDetectionConfig represents turn detection settings.
type TurnDetectionConfig struct {
	Type              string  `json:"type"`
	Threshold         float64 `json:"threshold,omitempty"`
	PrefixPaddingMs   int     `json:"prefix_padding_ms,omitempty"`
	SilenceDurationMs int     `json:"silence_duration_ms,omitempty"`
}

// Tool represents a tool or function that can be used by the assistant.
type Tool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  ToolParameters `json:"parameters"`
}

// ToolParameters represents the parameters for a tool.
type ToolParameters struct {
	Type       string                           `json:"type"`
	Properties map[string]ToolParameterProperty `json:"properties"`
	Required   []string                         `json:"required"`
}

// ToolParameterProperty represents a property of a tool parameter.
type ToolParameterProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Item represents an item in the conversation.
type Item struct {
	ID      string        `json:"id"`
	Object  string        `json:"object,omitempty"`
	Type    string        `json:"type"`
	Status  string        `json:"status,omitempty"`
	Role    string        `json:"role"`
	Content []ContentPart `json:"content"`
}

func (i Item) String() string {
	var content strings.Builder
	for _, part := range i.Content {
		switch part.Type {
		case "text", "input_text":
			if part.Text != nil {
				content.WriteString(*part.Text)
			}
		case "audio_transcript":
			if part.Transcript != nil {
				content.WriteString(*part.Transcript)
			}
		}
	}
	return content.String()
}

// ContentPart represents a content part within an item.
type ContentPart struct {
	Type       string  `json:"type"`
	Text       *string `json:"text,omitempty"`
	Transcript *string `json:"transcript,omitempty"`
	// Additional fields can be added here
}

// ResponseConfig represents the response configuration sent from the client.
type ResponseConfig struct {
	Modalities        []string `json:"modalities,omitempty"`
	Instructions      string   `json:"instructions,omitempty"`
	Voice             string   `json:"voice,omitempty"`
	OutputAudioFormat string   `json:"output_audio_format,omitempty"`
	Tools             []Tool   `json:"tools,omitempty"`
	ToolChoice        string   `json:"tool_choice,omitempty"`
	Temperature       float64  `json:"temperature,omitempty"`
	MaxOutputTokens   int      `json:"max_output_tokens,omitempty"`
}

// ResponseResource represents a response from the server.
type ResponseResource struct {
	ID            string      `json:"id"`
	Object        string      `json:"object"`
	Status        string      `json:"status"`
	StatusDetails interface{} `json:"status_details"`
	Output        []Item      `json:"output"`
	Usage         *Usage      `json:"usage"`
}

// Usage represents usage statistics for a response.
type Usage struct {
	TotalTokens        int                `json:"total_tokens"`
	InputTokens        int                `json:"input_tokens"`
	OutputTokens       int                `json:"output_tokens"`
	InputTokenDetails  InputTokenDetails  `json:"input_token_details"`
	OutputTokenDetails OutputTokenDetails `json:"output_token_details"`
}

// InputTokenDetails represents detailed input token usage.
type InputTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
	TextTokens   int `json:"text_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}

// OutputTokenDetails represents detailed output token usage.
type OutputTokenDetails struct {
	TextTokens  int `json:"text_tokens"`
	AudioTokens int `json:"audio_tokens"`
}

// Conversation represents a conversation resource.
type Conversation struct {
	ID     string `json:"id"`
	Object string `json:"object"`
}

// RateLimit represents rate limit information.
type RateLimit struct {
	Name         string  `json:"name"`
	Limit        int     `json:"limit"`
	Remaining    int     `json:"remaining"`
	ResetSeconds float64 `json:"reset_seconds"`
}

// -----------------------------------
// Event Type Mapping and Parsing
// -----------------------------------

// eventTypes maps event types to their corresponding struct instances.
var eventTypes = map[string]Event{
	// Client Events
	"session.update":             &SessionUpdateEvent{},
	"input_audio_buffer.append":  &InputAudioBufferAppendEvent{},
	"input_audio_buffer.commit":  &InputAudioBufferCommitEvent{},
	"input_audio_buffer.clear":   &InputAudioBufferClearEvent{},
	"conversation.item.create":   &ConversationItemCreateEvent{},
	"conversation.item.truncate": &ConversationItemTruncateEvent{},
	"conversation.item.delete":   &ConversationItemDeleteEvent{},
	"response.create":            &ResponseCreateEvent{},
	"response.cancel":            &ResponseCancelEvent{},
	// Server Events
	"error":                                                 &ErrorEvent{},
	"session.created":                                       &SessionCreatedEvent{},
	"session.updated":                                       &SessionUpdatedEvent{},
	"conversation.created":                                  &ConversationCreatedEvent{},
	"input_audio_buffer.committed":                          &InputAudioBufferCommittedEvent{},
	"input_audio_buffer.cleared":                            &InputAudioBufferClearedEvent{},
	"input_audio_buffer.speech_started":                     &InputAudioBufferSpeechStartedEvent{},
	"input_audio_buffer.speech_stopped":                     &InputAudioBufferSpeechStoppedEvent{},
	"conversation.item.created":                             &ConversationItemCreatedEvent{},
	"conversation.item.input_audio_transcription.completed": &ConversationItemInputAudioTranscriptionCompletedEvent{},
	"conversation.item.input_audio_transcription.failed":    &ConversationItemInputAudioTranscriptionFailedEvent{},
	"conversation.item.truncated":                           &ConversationItemTruncatedEvent{},
	"conversation.item.deleted":                             &ConversationItemDeletedEvent{},
	"response.created":                                      &ResponseCreatedEvent{},
	"response.done":                                         &ResponseDoneEvent{},
	"response.output_item.added":                            &ResponseOutputItemAddedEvent{},
	"response.output_item.done":                             &ResponseOutputItemDoneEvent{},
	"response.content_part.added":                           &ResponseContentPartAddedEvent{},
	"response.content_part.done":                            &ResponseContentPartDoneEvent{},
	"response.text.delta":                                   &ResponseTextDeltaEvent{},
	"response.text.done":                                    &ResponseTextDoneEvent{},
	"response.audio_transcript.delta":                       &ResponseAudioTranscriptDeltaEvent{},
	"response.audio_transcript.done":                        &ResponseAudioTranscriptDoneEvent{},
	"response.audio.delta":                                  &ResponseAudioDeltaEvent{},
	"response.audio.done":                                   &ResponseAudioDoneEvent{},
	"response.function_call_arguments.delta":                &ResponseFunctionCallArgumentsDeltaEvent{},
	"response.function_call_arguments.done":                 &ResponseFunctionCallArgumentsDoneEvent{},
	"rate_limits.updated":                                   &RateLimitsUpdatedEvent{},
}

var (
	eventTypeToReflectType map[string]reflect.Type
	reflectTypeToEventType map[reflect.Type]string
)

func init() {
	eventTypeToReflectType = make(map[string]reflect.Type)
	reflectTypeToEventType = make(map[reflect.Type]string)

	for eventType, eventInstance := range eventTypes {
		reflectType := reflect.TypeOf(eventInstance).Elem()
		eventTypeToReflectType[eventType] = reflectType
		reflectTypeToEventType[reflectType] = eventType
	}
}

// ParseEvent parses the JSON data and returns the appropriate Event struct.
func ParseEvent(data []byte) (Event, error) {
	// First, unmarshal into BaseEvent to get the Type
	var base BaseEvent
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, fmt.Errorf("error unmarshalling BaseEvent: %w", err)
	}

	reflectType, ok := eventTypeToReflectType[base.Type]
	if !ok {
		return nil, fmt.Errorf("unknown event type: %s", base.Type)
	}

	// Create a new pointer to the appropriate struct
	event := reflect.New(reflectType).Interface().(Event)
	if err := json.Unmarshal(data, event); err != nil {
		return nil, fmt.Errorf("error unmarshalling event %s: %w", base.Type, err)
	}

	return event, nil
}

// MarshalJSON is a custom JSON marshaler for Event types.
func MarshalJSON(e Event) ([]byte, error) {
	reflectType := reflect.TypeOf(e).Elem()
	eventType, ok := reflectTypeToEventType[reflectType]
	if !ok {
		return nil, fmt.Errorf("unknown event type: %T", e)
	}
	e.setType(eventType)
	return json.Marshal(e)
}
