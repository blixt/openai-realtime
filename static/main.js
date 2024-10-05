import htm from "htm";
import React, { useReducer, useCallback, useRef, useEffect } from "react";
import ReactDOM from "react-dom/client";
import { RealtimeManager } from "./RealtimeManager.js";
import { MESSAGE_TYPES } from "./messageTypes.js";
import { initialState, reducer } from "./reducer.js";

// In VSCode, use the lit-html extension for syntax highlighting.
const html = htm.bind(React.createElement);

function App() {
	const [state, dispatch] = useReducer(reducer, initialState);
	const { messages, error, usage, session, connectionStatus } = state;

	/** @type {React.RefObject<RealtimeManager | null>} */
	const realtimeManagerRef = useRef(null);

	useEffect(() => {
		realtimeManagerRef.current = new RealtimeManager(`ws://${window.location.host}/ws`, dispatch);

		return () => {
			if (realtimeManagerRef.current) {
				realtimeManagerRef.current.close();
				realtimeManagerRef.current = null;
			}
		};
	}, []);

	// Register functions once when session is available.
	const hasSession = !!session;
	useEffect(() => {
		if (!hasSession) return;
		const manager = realtimeManagerRef.current;
		if (!manager) return;

		manager.registerFunction({
			name: "change_background_color",
			description: "Change the background color of the page",
			parameters: {
				color: {
					type: "string",
					description: "The CSS color to change the background to",
					required: true,
				},
			},
			handler: (args) => {
				document.body.style.backgroundColor = args.color;
				console.log(`Background color changed to ${args.color}`);
			},
		});
	}, [hasSession]);

	const start = useCallback(async () => {
		try {
			await realtimeManagerRef.current.connect();
		} catch (error) {
			console.error("Failed to connect:", error);
		}
	}, []);

	const stop = useCallback(() => {
		realtimeManagerRef.current?.close();
		realtimeManagerRef.current = null;
	}, []);

	const sendTextMessage = useCallback((text) => {
		if (!realtimeManagerRef.current) {
			throw new Error("RealtimeManager is not initialized");
		}
		const messageId = Date.now().toString();
		realtimeManagerRef.current.send({
			type: MESSAGE_TYPES.MESSAGE_UPDATE,
			message_id: messageId,
			content: text,
		});
		dispatch({
			type: "UPDATE_MESSAGE",
			id: messageId,
			content: text,
			sender: "user",
			audio: false,
		});
	}, []);

	const handleSubmit = useCallback(
		(e) => {
			e.preventDefault();
			const text = e.target.elements.textInput.value.trim();
			if (text !== "") {
				sendTextMessage(text);
				e.target.elements.textInput.value = "";
			}
		},
		[sendTextMessage],
	);

	return html`
		<div className="container mx-auto p-4 max-w-2xl">
			<h1 className="text-2xl font-bold mb-4 text-gray-900 dark:text-gray-100">Realtime AI Assistant</h1>
			<div className="mb-4 space-x-2">
				<button
					onClick=${start}
					disabled=${connectionStatus !== "disconnected"}
					className="bg-blue-500 text-white font-bold py-2 px-4 rounded disabled:opacity-50 disabled:cursor-not-allowed"
				>
					${connectionStatus === "connecting" ? "Connecting..." : "Start"}
				</button>
				<button
					onClick=${stop}
					disabled=${connectionStatus !== "connected"}
					className="bg-red-500 text-white font-bold py-2 px-4 rounded disabled:opacity-50 disabled:cursor-not-allowed"
				>
					Stop
				</button>
			</div>

			${
				connectionStatus === "connecting" &&
				html`
				<div className="mb-4 text-gray-600 dark:text-gray-400">
					<p>Establishing connection... Please wait.</p>
					<div className="mt-2 w-8 h-8 border-t-2 border-b-2 border-gray-900 dark:border-gray-100 rounded-full animate-spin"></div>
				</div>
			`
			}

			${
				session &&
				html`
				<div className="mb-2 text-sm text-gray-600 dark:text-gray-400">
					<p>Model: ${session.model}</p>
					<p>Voice: ${session.voice}</p>
					<p>Input format: ${session.input_audio_format}</p>
					<p>Output format: ${session.output_audio_format}</p>
				</div>
			`
			}

			<div className="mb-2 text-sm text-gray-600 dark:text-gray-400">
				<p>Total Tokens: ${usage.total_tokens}</p>
				<p>Input Tokens: ${usage.input_tokens} (${usage.input_token_details.audio_tokens} audio + ${usage.input_token_details.text_tokens} text + ${usage.input_token_details.cached_tokens} cached)</p>
				<p>Output Tokens: ${usage.output_tokens} (${usage.output_token_details.audio_tokens} audio + ${usage.output_token_details.text_tokens} text)</p>
			</div>

			<div className="border border-gray-300 dark:border-gray-600 rounded p-4 mb-4 h-80 overflow-y-auto bg-white dark:bg-gray-800">
				${messages.map(
					(message) => html`
						<div key=${message.id} className=${`mb-2 ${message.sender === "user" ? "text-right" : "text-left"}`}>
							<p className=${`inline-block p-2 rounded-lg ${
								message.sender === "user"
									? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-100"
									: message.audio
										? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-100"
										: "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
							}`}>
								${message.audio ? "🎤 " : ""}${message.content}
								${message.isDone === false ? html`<span className="ml-2 animate-pulse">...</span>` : ""}
							</p>
						</div>
					`,
				)}
			</div>

			${error && html`<div className="text-red-500 mb-4">${error}</div>`}

			<form onSubmit=${handleSubmit} className="flex space-x-2">
				<input
					type="text"
					name="textInput"
					placeholder="Type your message here..."
					autoComplete="off"
					required
					disabled=${!session}
					className="flex-grow p-2 border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 disabled:opacity-50 disabled:cursor-not-allowed"
				/>
				<button
					type="submit"
					disabled=${!session}
					className="bg-green-500 text-white font-bold py-2 px-4 rounded disabled:opacity-50 disabled:cursor-not-allowed"
				>
					Send
				</button>
			</form>
		</div>
	`;
}

const root = ReactDOM.createRoot(document.getElementById("root"));
root.render(html`<${App} />`);
