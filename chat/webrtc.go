package chat

import (
	"encoding/base64"
	"log"

	"github.com/blixt/openai-realtime/openai"
	"github.com/pion/webrtc/v3"
)

// SetupWebRTC initializes the WebRTC connection for the user
func (user *User) SetupWebRTC(sdp string) error {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Println("Error creating PeerConnection:", err)
		return err
	}
	user.PeerConnection = peerConnection

	outputTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  1,
		},
		"audio",
		"pion",
	)
	if err != nil {
		log.Println("Error creating track:", err)
		return err
	}

	_, err = peerConnection.AddTrack(outputTrack)
	if err != nil {
		log.Println("Error adding track:", err)
		return err
	}
	user.OutputTrack = outputTrack

	peerConnection.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate != nil {
			candidateJSON := candidate.ToJSON()
			msg := &WebRTCCandidateMessage{
				Candidate:        candidateJSON.Candidate,
				SDPMid:           candidateJSON.SDPMid,
				SDPMLineIndex:    candidateJSON.SDPMLineIndex,
				UsernameFragment: candidateJSON.UsernameFragment,
			}
			user.WriteChan <- msg
		}
	})

	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("New track: %s\n", track.Codec().MimeType)

		if track.Kind() == webrtc.RTPCodecTypeAudio {
			go user.handleIncomingAudio(track)
		}
	})

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		log.Println("SetRemoteDescription error:", err)
		return err
	}

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		log.Println("CreateAnswer error:", err)
		return err
	}

	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		log.Println("SetLocalDescription error:", err)
		return err
	}

	<-gatherComplete

	msg := &WebRTCAnswerMessage{
		SDP: peerConnection.LocalDescription().SDP,
	}
	user.WriteChan <- msg

	return nil
}

// handleIncomingAudio processes incoming audio from the WebRTC track
func (user *User) handleIncomingAudio(track *webrtc.TrackRemote) {
	// TODO: Handle multiple clients. Currently, this will only work for a single client.
	// We need to implement a strategy for mixing audio from multiple clients or choosing a primary speaker.

	for {
		// Read RTP packets from the track
		rtp, _, err := track.ReadRTP()
		if err != nil {
			log.Println("Error reading RTP packet:", err)
			return
		}

		// Convert RTP packet to raw audio data
		samples := make([]byte, len(rtp.Payload))
		copy(samples, rtp.Payload)

		// Encode the audio data to base64
		encodedAudio := base64.StdEncoding.EncodeToString(samples)

		// Create an InputAudioBufferAppendEvent
		appendEvent := &openai.InputAudioBufferAppendEvent{
			BaseEvent: openai.BaseEvent{Type: "input_audio_buffer.append"},
			Audio:     encodedAudio,
		}

		// Send the event to OpenAI via the OpenAIWriteCh
		user.OpenAIWriteCh <- appendEvent
	}
}

// HandleICECandidate adds an ICE candidate to the peer connection
func (user *User) HandleICECandidate(candidate string, sdpMid *string, sdpMLineIndex *uint16, usernameFragment *string) error {
	iceCandidate := webrtc.ICECandidateInit{
		Candidate:        candidate,
		SDPMid:           sdpMid,
		SDPMLineIndex:    sdpMLineIndex,
		UsernameFragment: usernameFragment,
	}
	err := user.PeerConnection.AddICECandidate(iceCandidate)
	if err != nil {
		log.Println("AddICECandidate error:", err)
		return err
	}
	return nil
}
