import * as sdpTransform from "sdp-transform";
import { MESSAGE_TYPES } from "./messageTypes.js";

/**
 * @typedef {Object} SessionConfig
 * @property {string} instructions
 * @property {string} voice
 * @property {string} input_audio_format
 * @property {string} output_audio_format
 */

/**
 * @typedef {Object} FunctionDefinition
 * @property {Object} schema
 * @property {string} schema.type
 * @property {string} schema.name
 * @property {string} schema.description
 * @property {Object} schema.parameters
 * @property {function} handler
 */

/**
 * Manages real-time communication with a WebSocket server and WebRTC peer connection.
 */
export class RealtimeManager {
	#url;
	/** @type {WebSocket|null} */
	#ws = null;
	/** @type {RTCPeerConnection|null} */
	#pc = null;
	/** @type {MediaStream|null} */
	#localStream = null;
	/** @type {Object.<string, FunctionDefinition>} */
	#registeredFunctions = {};
	/** @type {SessionConfig|null} */
	#sessionConfig = null;
	/** @type {boolean} */
	#isClosed = true;
	/** @type {AudioContext|null} */
	#audioContext = null;
	/** @type {MediaStreamAudioSourceNode|null} */
	#microphoneSource = null;
	/** @type {MediaStreamAudioSourceNode|null} */
	#speakerSource = null;
	#dispatch = null;
	/** @type {RTCIceCandidate[]} */
	#iceCandidateBuffer = [];

	/**
	 * @param {string} url - The WebSocket URL to connect to
	 * @param {function} dispatch - The dispatch function to handle events
	 */
	constructor(url, dispatch) {
		this.#url = url;
		this.#dispatch = dispatch;
	}

	/**
	 * Establishes the WebSocket and WebRTC connections.
	 * @returns {Promise<void>}
	 */
	async connect() {
		if (!this.#isClosed) {
			console.warn("RealtimeManager is already connected.");
			return;
		}

		try {
			this.#dispatch({ type: "START_CONNECTION" });
			await this.#setupWebSocket();
			await this.#setupPeerConnection();
			await this.#setupAudioInput();
			this.#isClosed = false;
			console.log("RealtimeManager connected successfully");
		} catch (error) {
			console.error("Error during connection setup:", error);
			this.#dispatch({ type: "CONNECTION_FAILED", error: error.message });
			await this.close();
			throw error;
		}
	}

	/**
	 * Sets up the WebSocket connection.
	 * @returns {Promise<void>}
	 */
	async #setupWebSocket() {
		this.#ws = new WebSocket(this.#url);
		this.#ws.onopen = () => {
			console.log("WebSocket connection opened");
		};
		this.#ws.onmessage = this.#handleMessage.bind(this);
		this.#ws.onerror = (error) => this.#handleFatalError(error);
		this.#ws.onclose = () => this.#handleFatalError(new Error("WebSocket closed unexpectedly"));
	}

	/**
	 * Sets up the WebRTC peer connection.
	 * @returns {Promise<void>}
	 */
	async #setupPeerConnection() {
		this.#pc = new RTCPeerConnection();

		this.#pc.onicecandidate = this.#handleIceCandidate.bind(this);
		this.#pc.ontrack = (event) => {
			this.#setupAudioOutput(event.streams[0]);
		};
		this.#pc.oniceconnectionstatechange = () => {
			console.log("ICE connection state changed:", this.#pc.iceConnectionState);
			if (this.#pc.iceConnectionState === "failed") {
				this.#handleFatalError(new Error("ICE connection failed"));
			}
		};
		this.#pc.onsignalingstatechange = () => {
			console.log("Signaling state changed:", this.#pc.signalingState);
		};

		this.#pc.addTransceiver("audio", {
			direction: "sendrecv",
			sendEncodings: [],
		});

		const offer = await this.#createOffer();
		this.send({
			type: MESSAGE_TYPES.WEBRTC_OFFER,
			sdp: offer.sdp,
		});
	}

	/**
	 * Creates an offer for the WebRTC connection.
	 * @returns {Promise<RTCSessionDescriptionInit>}
	 */
	async #createOffer() {
		const offer = await this.#pc.createOffer();
		offer.sdp = this.#setCodecPreferences(offer.sdp);
		await this.#pc.setLocalDescription(offer);
		return this.#pc.localDescription;
	}

	/**
	 * Sets codec preferences in the SDP.
	 * @param {string} sdp - The Session Description Protocol string
	 * @returns {string} - The modified SDP string
	 */
	#setCodecPreferences(sdp) {
		console.log("Setting codec preferences");
		// OpenAI requires G.711 μ-law (PCMU) codec at 8kHz
		const parsedSdp = sdpTransform.parse(sdp);

		// Find the audio media section
		const audioMedia = parsedSdp.media.find((m) => m.type === "audio");
		if (!audioMedia) {
			console.warn("No audio media section found in SDP");
			return sdp;
		}

		// Find the PCMU codec
		const pcmuRtp = audioMedia.rtp.find((rtp) => rtp.codec.toUpperCase() === "PCMU" && rtp.rate === 8000);
		if (!pcmuRtp) {
			console.warn("PCMU codec not found in SDP");
			return sdp;
		}

		// Move PCMU to the first position in the payload list
		const pcmuPayload = pcmuRtp.payload.toString();
		const payloads = sdpTransform.parsePayloads(audioMedia.payloads);
		audioMedia.payloads = [pcmuPayload, ...payloads.filter((p) => p.toString() !== pcmuPayload)].join(" ");

		// Keep all codecs, but ensure PCMU is first
		audioMedia.rtp = [pcmuRtp, ...audioMedia.rtp.filter((rtp) => rtp.payload !== pcmuRtp.payload)];

		// Update fmtp and rtcp-fb entries
		if (audioMedia.fmtp) {
			audioMedia.fmtp = audioMedia.fmtp.filter((fmtp) => fmtp.payload === pcmuRtp.payload || payloads.includes(fmtp.payload));
		}
		if (audioMedia.rtcpFb) {
			audioMedia.rtcpFb = audioMedia.rtcpFb.filter((rtcpFb) => rtcpFb.payload === pcmuRtp.payload || payloads.includes(rtcpFb.payload));
		}

		const modifiedSdp = sdpTransform.write(parsedSdp);
		console.log("SDP after setting codec preferences:", modifiedSdp);
		return modifiedSdp;
	}

	/**
	 * Handles ICE candidate events.
	 * @param {RTCPeerConnectionIceEvent} event
	 */
	#handleIceCandidate(event) {
		if (event.candidate) {
			const candidate = {
				type: MESSAGE_TYPES.WEBRTC_CANDIDATE,
				candidate: event.candidate.candidate,
				sdpMid: event.candidate.sdpMid,
				sdpMLineIndex: event.candidate.sdpMLineIndex,
			};
			this.send(candidate);
		}
	}

	/**
	 * Handles incoming WebSocket messages.
	 * @param {MessageEvent} event
	 */
	async #handleMessage(event) {
		try {
			const msg = JSON.parse(event.data);

			switch (msg.type) {
				case MESSAGE_TYPES.SESSION_UPDATE:
					this.#sessionConfig = msg.session;
					this.#dispatch({ type: "SESSION_CHANGED", session: this.#sessionConfig });
					console.log("Session config updated:", this.#sessionConfig);
					break;
				case MESSAGE_TYPES.ERROR:
					this.#dispatch({ type: "ERROR", error: msg.error });
					break;
				case MESSAGE_TYPES.WEBRTC_ANSWER:
					await this.#handleWebRTCAnswer(msg);
					break;
				case MESSAGE_TYPES.WEBRTC_CANDIDATE:
					await this.#handleWebRTCCandidate(msg);
					break;
				case MESSAGE_TYPES.MESSAGE_UPDATE:
					this.#dispatch({
						type: "UPDATE_MESSAGE",
						id: msg.message_id,
						patch: {
							content: msg.content,
							sender: msg.sender,
							audio: msg.audio,
							isDone: msg.is_done,
						},
					});
					break;
				case MESSAGE_TYPES.MESSAGE_DELTA:
					this.#dispatch({
						type: "APPEND_MESSAGE",
						id: msg.message_id,
						content: msg.content,
					});
					break;
				case MESSAGE_TYPES.FUNCTION_EXECUTE:
					this.#handleFunctionExecution(msg);
					break;
				case MESSAGE_TYPES.TOKEN_UPDATE:
					this.#dispatch({
						type: "UPDATE_TOKENS",
						usage: msg.usage,
					});
					break;
				default:
					console.log("Unhandled message type:", msg.type);
			}
		} catch (error) {
			console.error("Error handling WebSocket message:", error);
			this.#handleFatalError(error);
		}
	}

	/**
	 * Handles WebRTC answer messages.
	 * @param {Object} msg - The answer message
	 * @returns {Promise<void>}
	 */
	async #handleWebRTCAnswer(msg) {
		const remoteDesc = new RTCSessionDescription({
			type: "answer",
			sdp: msg.sdp,
		});
		await this.#pc.setRemoteDescription(remoteDesc);

		// After setting the remote description, add any buffered candidates
		while (this.#iceCandidateBuffer.length > 0) {
			const candidate = this.#iceCandidateBuffer.shift();
			await this.#pc.addIceCandidate(candidate);
		}
	}

	/**
	 * Handles WebRTC candidate messages.
	 * @param {Object} msg - The candidate message
	 * @returns {Promise<void>}
	 */
	async #handleWebRTCCandidate(msg) {
		const candidate = new RTCIceCandidate(msg);

		if (this.#pc.remoteDescription) {
			try {
				await this.#pc.addIceCandidate(candidate);
			} catch (e) {
				console.error("Error adding received ICE candidate:", e);
			}
		} else {
			// If the remote description is not set yet, buffer the candidate
			this.#iceCandidateBuffer.push(candidate);
		}
	}

	/**
	 * Handles function execution messages.
	 * @param {Object} msg - The function execution message
	 */
	#handleFunctionExecution(msg) {
		const functionName = msg.name;
		const args = JSON.parse(msg.arguments);
		const func = this.#registeredFunctions[functionName];
		if (func) {
			const result = func.handler(args);
			console.log(`Executed function ${functionName} with args:`, args);

			this.send({
				type: MESSAGE_TYPES.FUNCTION_RESULT,
				name: functionName,
				result,
			});
		} else {
			console.error(`Function ${functionName} is not registered.`);
		}
	}

	/**
	 * Registers a function that can be called by the server.
	 * @param {Object} options
	 * @param {string} options.name - The name of the function
	 * @param {string} options.description - A description of the function
	 * @param {Object} options.parameters - The parameters of the function
	 * @param {function} options.handler - The function to be executed
	 */
	registerFunction({ name, description, parameters, handler }) {
		const properties = {};
		const required = [];

		for (const [key, value] of Object.entries(parameters)) {
			const { required: isRequired, ...cleanedValue } = value;
			properties[key] = cleanedValue;
			if (isRequired) {
				required.push(key);
			}
		}

		const functionDefinition = {
			schema: {
				type: "function",
				name,
				description,
				parameters: {
					type: "object",
					properties,
					required,
				},
			},
			handler,
		};

		this.#registeredFunctions[name] = functionDefinition;
		this.#sendFunctionRegistration(functionDefinition);
	}

	/**
	 * Sends a function registration message to the server.
	 * @param {FunctionDefinition} functionDefinition
	 */
	#sendFunctionRegistration(functionDefinition) {
		const payload = {
			type: MESSAGE_TYPES.FUNCTION_ADD,
			schema: functionDefinition.schema,
		};
		this.send(payload);
	}

	/**
	 * Sends a message through the WebSocket connection.
	 * @param {Object} message - The message to send
	 */
	send(message) {
		if (this.#ws && this.#ws.readyState === WebSocket.OPEN) {
			this.#ws.send(JSON.stringify(message));
		} else {
			console.error("WebSocket is not open. Cannot send message.");
		}
	}

	/**
	 * Closes all connections and cleans up resources.
	 * @returns {Promise<void>}
	 */
	async close() {
		if (this.#isClosed) return;
		this.#isClosed = true;

		console.log("Closing RealtimeManager");

		// Cleanup PeerConnection
		if (this.#pc) {
			this.#pc.onicecandidate = null;
			this.#pc.ontrack = null;
			this.#pc.oniceconnectionstatechange = null;
			this.#pc.onsignalingstatechange = null;
			this.#pc.close();
			this.#pc = null;
		}

		// Cleanup Audio Streams
		if (this.#localStream) {
			for (const track of this.#localStream.getTracks()) {
				track.stop();
			}
			this.#localStream = null;
		}

		if (this.#speakerSource) {
			this.#speakerSource.disconnect();
			this.#speakerSource = null;
		}

		if (this.#microphoneSource) {
			this.#microphoneSource.disconnect();
			this.#microphoneSource = null;
		}

		if (this.#audioContext) {
			await this.#audioContext.close();
			this.#audioContext = null;
		}

		// Cleanup WebSocket
		if (this.#ws) {
			this.#ws.onopen = null;
			this.#ws.onmessage = null;
			this.#ws.onerror = null;
			this.#ws.onclose = null;
			if (this.#ws.readyState === WebSocket.OPEN || this.#ws.readyState === WebSocket.CONNECTING) {
				this.#ws.close();
			}
			this.#ws = null;
		}

		this.#dispatch({ type: "END_CONNECTION" });
	}

	#handleFatalError(error) {
		console.error("Fatal error occurred:", error);
		this.#dispatch({ type: "CONNECTION_FAILED", error: error.message });
		this.close();
	}

	/**
	 * Sets up the audio input (microphone) for the WebRTC connection.
	 * @returns {Promise<void>}
	 */
	async #setupAudioInput() {
		try {
			this.#localStream = await navigator.mediaDevices.getUserMedia({ audio: true });

			if (!this.#audioContext) {
				this.#audioContext = new (window.AudioContext || window.webkitAudioContext)();
			}

			this.#microphoneSource = this.#audioContext.createMediaStreamSource(this.#localStream);

			// Connect the microphone source to the peer connection
			for (const track of this.#localStream.getTracks()) {
				this.#pc.addTrack(track, this.#localStream);
			}

			console.log("Audio input set up successfully");
		} catch (error) {
			console.error("Error accessing microphone:", error);
			throw new Error("Failed to set up audio input");
		}
	}

	/**
	 * Sets up the audio output (speaker) for the WebRTC connection.
	 * @param {MediaStream} remoteStream - The remote audio stream
	 */
	#setupAudioOutput(remoteStream) {
		try {
			if (!this.#audioContext) {
				this.#audioContext = new (window.AudioContext || window.webkitAudioContext)();
			}

			this.#speakerSource = this.#audioContext.createMediaStreamSource(remoteStream);
			this.#speakerSource.connect(this.#audioContext.destination);
			console.log("Audio output set up successfully");
		} catch (error) {
			console.error("Error setting up audio output:", error);
			this.#handleFatalError(new Error("Failed to set up audio output"));
		}
	}

	/**
	 * Sends a text message through the WebSocket connection.
	 * @param {string} text - The text message to send
	 */
	sendTextMessage(text) {
		if (!this.#ws) {
			throw new Error("WebSocket is not initialized");
		}
		const messageId = Date.now().toString();
		this.send({
			type: MESSAGE_TYPES.MESSAGE_UPDATE,
			message_id: messageId,
			content: text,
			sender: "user",
			is_done: true,
		});
		this.#dispatch({
			type: "UPDATE_MESSAGE",
			id: messageId,
			patch: {
				content: text,
				sender: "user",
				isDone: true,
			},
		});
	}
}
