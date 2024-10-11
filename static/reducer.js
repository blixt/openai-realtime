import { produce } from "immer";

/**
 * @typedef {Object} Message
 * @property {string} id
 * @property {string} content
 * @property {string} sender
 * @property {boolean} isComplete
 * @property {boolean} audio
 * @property {boolean} isDone
 */

/**
 * @typedef {Object} Tool
 * @property {string} description
 * @property {string} name
 * @property {Object} parameters
 * @property {string} type
 */

/**
 * @typedef {Object} TurnDetection
 * @property {number} prefix_padding_ms
 * @property {number} silence_duration_ms
 * @property {number} threshold
 * @property {string} type
 */

/**
 * @typedef {Object} Session
 * @property {number} expires_at
 * @property {string} id
 * @property {string} input_audio_format
 * @property {string|null} input_audio_transcription
 * @property {string} instructions
 * @property {string|number} max_response_output_tokens
 * @property {string[]} modalities
 * @property {string} model
 * @property {string} object
 * @property {string} output_audio_format
 * @property {number} temperature
 * @property {string} tool_choice
 * @property {Tool[]} tools
 * @property {TurnDetection} turn_detection
 * @property {string} voice
 */

/**
 * @typedef {Object} InputTokenDetails
 * @property {number} cached_tokens
 * @property {number} text_tokens
 * @property {number} audio_tokens
 */

/**
 * @typedef {Object} OutputTokenDetails
 * @property {number} text_tokens
 * @property {number} audio_tokens
 */

/**
 * @typedef {Object} Usage
 * @property {number} total_tokens
 * @property {number} input_tokens
 * @property {number} output_tokens
 * @property {InputTokenDetails} input_token_details
 * @property {OutputTokenDetails} output_token_details
 */

/**
 * @typedef {Object} State
 * @property {Message[]} messages
 * @property {string|null} error
 * @property {Usage} usage
 * @property {Session|null} session
 * @property {'disconnected'|'connecting'|'connected'} connectionStatus
 */

/** @type {State} */
export const initialState = {
    messages: [],
    error: null,
    usage: {
        total_tokens: 0,
        input_tokens: 0,
        output_tokens: 0,
        input_token_details: {
            cached_tokens: 0,
            text_tokens: 0,
            audio_tokens: 0,
        },
        output_token_details: {
            text_tokens: 0,
            audio_tokens: 0,
        },
    },
    session: null,
    connectionStatus: "disconnected",
};

/**
 * @typedef {(
 *   | { type: "START_CONNECTION" }
 *   | { type: "SESSION_CHANGED"; session: Session }
 *   | { type: "CONNECTION_FAILED"; error: string }
 *   | { type: "END_CONNECTION" }
 *   | { type: "UPDATE_MESSAGE"; id: string; patch: Partial<Message> }
 *   | { type: "APPEND_MESSAGE"; id: string; content: string }
 *   | { type: "UPDATE_TOKENS"; usage: Usage }
 *   | { type: "ERROR"; error: string }
 * )} Action
 */

/**
 * @param {State} state
 * @param {Action} action
 * @returns {State}
 */
export function reducer(state, action) {
    return produce(state, (draft) => {
        switch (action.type) {
            case "START_CONNECTION":
                draft.connectionStatus = "connecting";
                draft.error = null;
                draft.messages = [];
                break;

            case "SESSION_CHANGED":
                if (action.session) {
                    draft.connectionStatus = "connected";
                }
                draft.session = action.session;
                break;

            case "CONNECTION_FAILED":
                draft.connectionStatus = "disconnected";
                draft.error = action.error;
                break;

            case "END_CONNECTION":
                draft.connectionStatus = "disconnected";
                draft.session = null;
                break;

            case "UPDATE_MESSAGE": {
                const existingMessageIndex = draft.messages.findIndex((m) => m.id === action.id);
                if (existingMessageIndex !== -1) {
                    const existingMessage = draft.messages[existingMessageIndex];
                    Object.assign(existingMessage, action.patch);
                } else {
                    draft.messages.push({
                        id: action.id,
                        content: "",
                        sender: null,
                        audio: false,
                        isDone: false,
                        ...action.patch,
                    });
                }
                break;
            }

            case "APPEND_MESSAGE": {
                const messageIndex = draft.messages.findIndex((m) => m.id === action.id);
                if (messageIndex === -1) {
                    console.warn(`Message with id ${action.id} not found for append operation`);
                    break;
                }
                draft.messages[messageIndex].content += action.content;
                break;
            }

            case "UPDATE_TOKENS":
                draft.usage.total_tokens += action.usage.total_tokens;
                draft.usage.input_tokens += action.usage.input_tokens;
                draft.usage.output_tokens += action.usage.output_tokens;

                draft.usage.input_token_details.cached_tokens += action.usage.input_token_details.cached_tokens;
                draft.usage.input_token_details.text_tokens += action.usage.input_token_details.text_tokens;
                draft.usage.input_token_details.audio_tokens += action.usage.input_token_details.audio_tokens;

                draft.usage.output_token_details.text_tokens += action.usage.output_token_details.text_tokens;
                draft.usage.output_token_details.audio_tokens += action.usage.output_token_details.audio_tokens;
                break;

            case "ERROR":
                draft.error = action.error;
                break;

            default:
                console.warn(`Unhandled action type: ${action.type}`);
        }
    });
}
