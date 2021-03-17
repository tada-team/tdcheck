package checkers

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/pion/webrtc/v2"
	"github.com/pkg/errors"
	"github.com/tada-team/tdproto"
)

func NewCallsChecker() *callsChecker {
	p := new(callsChecker)
	p.do = p.doCheck
	p.Name = "tdcheck_calls"
	return p
}

type callsChecker struct {
	BaseUserChecker

	duration    time.Duration
	bobJid      tdproto.JID
	iceServer   string
	numTimeouts int
}

func (p *callsChecker) Report(w http.ResponseWriter) {
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

	answer, err := p.aliceWsSession.SendCallOffer(p.bobJid, offer.SDP)
	if err != nil {
		p.duration = p.Interval
		return errors.Wrap(err, "SendCallOffer fail")
	}
	log.Printf("[%s] %s: peer answer sent (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	if err := peerConnection.SetRemoteDescription(answer); err != nil {
		p.duration = p.Interval
		return errors.Wrap(err, "SetRemoteDescription fail")
	}
	log.Printf("[%s] %s: set remote description (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	if err := p.aliceWsSession.SendCallLeave(p.bobJid); err != nil {
		p.duration = p.Interval
		return err
	}
	log.Printf("[%s] %s: call leaved (%s)", p.Host, p.Name, time.Since(start).Round(time.Millisecond))

	p.duration = time.Since(start)
	log.Printf("[%s] %s: %s OK", p.Host, p.Name, p.duration.Round(time.Millisecond))

	return nil
}

func (p *callsChecker) newPeerConnection() (peerConnection *webrtc.PeerConnection, offer webrtc.SessionDescription, outputTrack *webrtc.Track, err error) {
	peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{
			URLs: []string{
				p.iceServer,
			},
		}},
	})
	if err != nil {
		return nil, offer, nil, err
	}

	var mediaEngine webrtc.MediaEngine
	mediaEngine.RegisterCodec(webrtc.NewRTPOpusCodec(webrtc.DefaultPayloadTypeOpus, 48000))

	// Add codecs
	audioCodecs := mediaEngine.GetCodecsByKind(webrtc.RTPCodecTypeAudio)
	if len(audioCodecs) == 0 {
		return nil, offer, nil, err
	}
	outputTrack, err = peerConnection.NewTrack(audioCodecs[0].PayloadType, rand.Uint32(), "audio", "pion")
	if err != nil {
		return nil, offer, nil, err
	}
	if _, err = peerConnection.AddTrack(outputTrack); err != nil {
		return nil, offer, nil, err
	}

	offer, err = peerConnection.CreateOffer(nil)
	if err != nil {
		return nil, offer, nil, err
	}

	err = mediaEngine.PopulateFromSDP(offer)
	if err != nil {
		return nil, offer, nil, err
	}

	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		return nil, offer, nil, err
	}

	// write output if program "hear" something
	peerConnection.OnTrack(func(track *webrtc.Track, receiver *webrtc.RTPReceiver) {
		log.Printf("[%s] %s got new track, id: %v\n", p.Host, p.Name, track.ID())
	})

	return peerConnection, offer, outputTrack, nil
}
