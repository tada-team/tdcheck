package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/pion/webrtc/v2"
	"github.com/pkg/errors"
	"github.com/tada-team/tdproto"
)

func NewCallsChecker() *callsChecker {
	p := new(callsChecker)
	p.do = p.doCheck
	p.Name = "calls_checker"
	return p
}

type callsChecker struct {
	BaseUserChecker
	duration    time.Duration
	bobJid      tdproto.JID
	iceServer   string
	numTimeouts int
}

func (p *callsChecker) Report(w io.Writer) {
	if p.Enabled() {
		_, _ = io.WriteString(w, "# TYPE tdcheck_calls_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls_ms{host=\"%s\"} %d\n", p.Host, p.duration.Milliseconds()))
	}
}

func (p *callsChecker) doCheck() error {
	if p.bobJid.Empty() || p.iceServer == "" {
		contact, err := p.bobSession.Me(p.Team)
		if err != nil {
			return err
		}
		p.bobJid = contact.Jid

		features, err := p.bobSession.Features()
		if err != nil {
			return err
		}
		p.iceServer = features.ICEServers[0].Urls

		// dont need for bob ws
		if err := p.bobWsSession.Close(); err != nil {
			return err
		}
	}

	start := time.Now()

	peerConnection, offer, _, err := p.newPeerConnection()
	if err != nil {
		p.duration = p.Interval
		return errors.Wrap(err, "NewPeerConnection fail")
	}
	log.Printf("[%s] %s: peer connection created (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	defer func() {
		if err := peerConnection.Close(); err != nil {
			log.Printf("[%s] %s: connection close fail: %s (%s).", p.Host, p.Name, err, time.Since(start).Round(time.Millisecond))
			return
		}
		log.Printf("[%s] %s: connection closed (%s).", p.Host, p.Name, time.Since(start).Round(time.Millisecond))
	}()

	p.aliceWsSession.SendCallOffer(p.bobJid, offer.SDP)
	log.Printf("[%s] %s: call offer sent (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	callAnswer := new(tdproto.ServerCallAnswer)
	if err := p.aliceWsSession.WaitFor(callAnswer); err != nil {
		p.duration = p.Interval
		return errors.Wrap(err, "ServerCallAnswer timeout")
	}
	log.Printf("[%s] %s: got call answer (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	if err := peerConnection.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  callAnswer.Params.JSEP.SDP,
	}); err != nil {
		p.duration = p.Interval
		return errors.Wrap(err, "SetRemoteDescription fail")
	}
	log.Printf("[%s] %s: set remote description (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	p.aliceWsSession.SendCallLeave(p.bobJid)
	log.Printf("[%s] %s: call leave sent (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	serverLeaveAnswer := new(tdproto.ServerCallLeave)
	if err := p.aliceWsSession.WaitFor(serverLeaveAnswer); err != nil {
		return err
	}
	log.Printf("[%s] %s: got server call leave (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	p.duration = time.Since(start)
	log.Printf("[%s] %s: ok (%s)", p.Host, p.Name, p.duration.Round(time.Millisecond))

	return nil
}

func (p *callsChecker) newPeerConnection() (*webrtc.PeerConnection, *webrtc.SessionDescription, *webrtc.Track, error) {
	peerConnection, err := webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{
			URLs: []string{
				p.iceServer,
			},
		}},
	})
	if err != nil {
		return nil, nil, nil, err
	}

	var mediaEngine webrtc.MediaEngine
	mediaEngine.RegisterCodec(webrtc.NewRTPOpusCodec(webrtc.DefaultPayloadTypeOpus, 48000))

	audioCodecs := mediaEngine.GetCodecsByKind(webrtc.RTPCodecTypeAudio)
	if len(audioCodecs) == 0 {
		return nil, nil, nil, err
	}

	outputTrack, err := peerConnection.NewTrack(audioCodecs[0].PayloadType, rand.Uint32(), "audio", "pion")
	if err != nil {
		return nil, nil, nil, err
	}

	if _, err := peerConnection.AddTrack(outputTrack); err != nil {
		return nil, nil, nil, err
	}

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := mediaEngine.PopulateFromSDP(offer); err != nil {
		return nil, nil, nil, err
	}

	if err := peerConnection.SetLocalDescription(offer); err != nil {
		return nil, nil, nil, err
	}

	// write output if program "hear" something
	peerConnection.OnTrack(func(track *webrtc.Track, receiver *webrtc.RTPReceiver) {
		log.Printf("[%s] %s: got new track, id: %v\n", p.Host, p.Name, track.ID())
	})

	return peerConnection, &offer, outputTrack, nil
}
