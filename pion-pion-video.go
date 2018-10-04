package main

import (
	"fmt"
	"time"
	//"math/rand"
	"github.com/pions/webrtc"
	gstsend "./gstsend"
	gstrecv "./gstrecv"
	//"github.com/pions/webrtc/pkg/datachannel"
	"github.com/pions/webrtc/pkg/ice"
	"github.com/pions/webrtc/pkg/rtcp"
)

const (
	rtcpPLIInterval = time.Second * 3
)

// check is used to panic in an error occurs.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

func offerer(offerSDP chan<- string, answerSDP <-chan string){
	// Setup the codecs you want to use.
	// We'll use the default ones but you can also define your own
	webrtc.RegisterDefaultCodecs()

	// Prepare the configuration
	config := webrtc.RTCConfiguration{
		IceServers: []webrtc.RTCIceServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.New(config)
	check(err)
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange = func(connectionState ice.ConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	}

	// Create a video track
	vp8Track, err := peerConnection.NewRTCTrack(webrtc.DefaultPayloadTypeVP8, "video", "pion2")
	if err != nil {
		panic(err)
	}
	_, err = peerConnection.AddTrack(vp8Track)
	if err != nil {
		panic(err)
	}

	// Create an offer to send to the browser
	offer, err := peerConnection.CreateOffer(nil)
	check(err)

	offerSDP <- offer.Sdp

	fmt.Println("offer: ", offer.Sdp)
	// Set the remote SessionDescription
	answer := webrtc.RTCSessionDescription{
		Type: webrtc.RTCSdpTypeAnswer,
		Sdp:  <-answerSDP,
	}
	fmt.Println("answer: ", answer.Sdp)
	// Apply the answer as the remote description
	err = peerConnection.SetRemoteDescription(answer)
	check(err)

	// Start pushing buffers on these tracks
	//gstsend.CreatePipeline(webrtc.Opus, opusTrack.Samples).Start()
	gstsend.CreatePipeline(webrtc.VP8, vp8Track.Samples).Start()
	select{}
}

func answerer( offerSDP <-chan string, answerSDP chan<- string){
	// Setup the codecs you want to use.
	// We'll use the default ones but you can also define your own
	webrtc.RegisterDefaultCodecs()
	// Prepare the configuration
	config := webrtc.RTCConfiguration{
		IceServers: []webrtc.RTCIceServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.New(config)
	check(err)
	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange = func(connectionState ice.ConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	}

	// Set a handler for when a new remote track starts, this handler creates a gstreamer pipeline
	// for the given codec
	peerConnection.OnTrack = func(track *webrtc.RTCTrack) {
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		// This is a temporary fix until we implement incoming RTCP events, then we would push a PLI only when a viewer requests it
		go func() {
			ticker := time.NewTicker(rtcpPLIInterval)
			for {
				select {
				case <-ticker.C:
					err := peerConnection.SendRTCP(&rtcp.PictureLossIndication{MediaSSRC: track.Ssrc})
					if err != nil {
						fmt.Println(err)
					}
				}
			}
		}()

		codec := track.Codec
		fmt.Printf("Track has started, of type %d: %s \n", track.PayloadType, codec.Name)
		pipeline := gstrecv.CreatePipeline(codec.Name)
		pipeline.Start()
		for {
			p := <-track.Packets
			//fmt.Print(".")
			pipeline.Push(p.Raw)
		}
	}
	
	fmt.Println("Get offerSDP")
	// Set the remote SessionDescription
	offer := webrtc.RTCSessionDescription{
		Type: webrtc.RTCSdpTypeOffer,
		Sdp:  <-offerSDP,
	}

	err = peerConnection.SetRemoteDescription(offer)
	check(err)

	// Sets the LocalDescription, and starts our UDP listeners
	answer, err := peerConnection.CreateAnswer(nil)
	check(err)

	fmt.Println("Send answerSDP")
	answerSDP <- answer.Sdp
	select{}
}

func main() {
	fmt.Println("hello")

	sdp1 := make(chan string,1)
	sdp2 := make(chan string,1)
	go offerer(sdp1, sdp2)
	go answerer(sdp1, sdp2)

	select{}
}