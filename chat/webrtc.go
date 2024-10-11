package chat

import (
	"encoding/base64"
	"fmt"
	"log"

	"github.com/blixt/openai-realtime/openai"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
)

// SetupWebRTC initializes the WebRTC connection for the user
func (user *User) SetupWebRTC(sdp string) error {
	// Create a MediaEngine object to configure the supported codec
	params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1, SDPFmtpLine: "", RTCPFeedback: nil},
		PayloadType:        0,
	}
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(params, webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}

	// Create a InterceptorRegistry
	i := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
		return err
	}

	// Create the API object with the MediaEngine
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithInterceptorRegistry(i))

	// Use the API to create the PeerConnection
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})
	if err != nil {
		log.Println("Error creating PeerConnection:", err)
		return err
	}
	user.PeerConnection = peerConnection

	outputTrack, err := webrtc.NewTrackLocalStaticRTP(
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

	rtpSender, err := peerConnection.AddTrack(outputTrack)
	if err != nil {
		log.Println("Error adding track:", err)
		return err
	}
	user.OutputTrack = outputTrack

	go func() {
		rtcpBuf := make([]byte, 1500)
		for {
			if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
				return
			}
		}
	}()

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
		// TODO: Handle video tracks when implementing video support
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

	// After setting up the PeerConnection and tracks
	// Set up the audio tracks for this user in the room
	if err = user.Room.setupUserWebRTC(user); err != nil {
		return fmt.Errorf("error setting up user WebRTC: %w", err)
	}

	return nil
}

// handleIncomingAudio processes incoming audio from the WebRTC track
func (user *User) handleIncomingAudio(track *webrtc.TrackRemote) {
	for {
		// Read RTP packets from the track
		rtp, _, err := track.ReadRTP()
		if err != nil {
			log.Println("Error reading RTP packet:", err)
			return
		}

		// Write the RTP packet to the output track
		if err = user.OutputTrack.WriteRTP(rtp); err != nil {
			log.Println("Error writing RTP packet:", err)
		}

		// Encode the audio data to base64
		encodedAudio := base64.StdEncoding.EncodeToString(rtp.Payload)

		// Create an InputAudioBufferAppendEvent
		appendEvent := &openai.InputAudioBufferAppendEvent{
			BaseEvent: openai.BaseEvent{Type: "input_audio_buffer.append"},
			Audio:     encodedAudio,
		}

		// Send the event to OpenAI via the Room's OpenAIWriteCh
		user.Room.openAIWriteCh <- appendEvent

		// Broadcast the audio to other users in the room
		user.Room.broadcastAudio(user, rtp)
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
