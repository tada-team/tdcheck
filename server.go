package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v2"
	"github.com/pkg/errors"
	"github.com/tada-team/tdclient"
	"github.com/tada-team/tdproto"

	"github.com/tada-team/tdcheck/checkers"
)

const (
	retryInterval = time.Second
	wsFailsCheck  = time.Minute
	maxTimeouts   = 10
	maxWsFails    = 120
)

var maxTimeoutsReached = errors.New("max timeouts")

func ServerWatch(s Server, rtr *mux.Router) {
	var wsFails int
	go func() {
		for range time.Tick(wsFailsCheck) {
			if wsFails > maxWsFails {
				log.Panicln("too many ws fails:", wsFails)
			}
			if wsFails > 0 {
				wsFails--
			}
		}
	}()

	var apiPing checkers.UrlChecker
	apiPing.Host = s.Host
	apiPing.Name = "tdcheck_api_ping_ms"
	apiPing.Path = "/api/v4/ping"
	apiPing.Interval = s.ApiPingInterval
	go apiPing.Start()

	var nginxPing checkers.UrlChecker
	nginxPing.Host = s.Host
	nginxPing.Name = "tdcheck_nginx_ping_ms"
	nginxPing.Path = "/ping.txt"
	nginxPing.Interval = s.NginxPingInterval
	go nginxPing.Start()

	var userverPing checkers.UrlChecker
	userverPing.Host = s.Host
	userverPing.Name = "tdcheck_userver_ping_ms"
	userverPing.Path = s.UServerPingPath
	userverPing.Interval = s.UServerPingInterval
	go userverPing.Start()

	wsPing := checkers.NewWsPingChecker()
	wsPing.Host = s.Host
	wsPing.Fails = &wsFails
	wsPing.Interval = s.WsPingInterval
	wsPing.Team = s.TestTeam
	wsPing.AliceToken = s.AliceToken
	wsPing.Verbose = s.Verbose
	go wsPing.Start()

	checkOnliners := checkers.NewOnlinersChecker()
	checkOnliners.Host = s.Host
	checkOnliners.Fails = &wsFails
	checkOnliners.Interval = s.MaxServerOnlineInterval
	checkOnliners.Team = s.TestTeam
	checkOnliners.AliceToken = s.AliceToken
	checkOnliners.Verbose = s.Verbose
	go checkOnliners.Start()

	checkMessage := checkers.NewMessageChecker()
	checkMessage.Host = s.Host
	checkMessage.Fails = &wsFails
	checkMessage.Interval = s.CheckMessageInterval
	checkMessage.Team = s.TestTeam
	checkMessage.AliceToken = s.AliceToken
	checkMessage.BobToken = s.BobToken
	checkMessage.Verbose = s.Verbose
	go checkMessage.Start()

	go s.checkCall()

	path := "/" + s.Host
	log.Println(
		"listen path:", path,
		"| api:", apiPing.Enabled(),
		"| nginx:", nginxPing.Enabled(),
		"| userver:", userverPing.Enabled(),
		"| ws ping:", wsPing.Enabled(),
		"| message:", checkMessage.Enabled(),
		"| calls:", s.checkCallEnabled(),
		"| onliners:", checkOnliners.Enabled(),
	)

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] request", s.Host)

		apiPing.Report(w)
		nginxPing.Report(w)
		userverPing.Report(w)
		wsPing.Report(w)
		checkOnliners.Report(w)
		checkMessage.Report(w)

		if s.checkCallEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_calls_fails counter\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls_fails{host=\"%s\"} %d\n", s.Host, s.callsFails))
			_, _ = io.WriteString(w, "# TYPE tdcheck_calls_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls_ms{host=\"%s\"} %d\n", s.Host, s.checkCallDuration.Milliseconds()))
		}

		_, _ = io.WriteString(w, "# TYPE tdcheck_ws_fails gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_fails{host=\"%s\"} %d\n", s.Host, wsFails))
	})
}

type Server struct {
	Host string `yaml:"host"`

	TestTeam   string `yaml:"test_team"`
	AliceToken string `yaml:"alice_token"`
	BobToken   string `yaml:"bob_token"`
	Verbose    bool   `yaml:"verbose"`

	ApiPingInterval time.Duration `yaml:"api_ping_interval"`

	NginxPingInterval       time.Duration `yaml:"nginx_ping_interval"`
	WsPingInterval          time.Duration `yaml:"ws_ping_interval"`
	MaxServerOnlineInterval time.Duration `yaml:"max_server_online_interval"`

	UServerPingInterval time.Duration `yaml:"userver_ping_interval"`
	UServerPingPath     string        `yaml:"userver_ping_path"`

	CheckMessageInterval time.Duration `yaml:"check_message_interval"`
	CheckCallInterval    time.Duration `yaml:"check_call_interval"`

	checkCallDuration    time.Duration
	callsFails           int
}

func (s *Server) tdClient(token string, timeout time.Duration) (*tdclient.Session, error) {
	sess, err := tdclient.NewSession(checkers.ForceScheme(s.Host))
	if err != nil {
		return nil, err
	}

	sess.Timeout = timeout
	sess.SetToken(token)
	sess.SetVerbose(s.Verbose)
	return &sess, nil
}

//func (s *Server) apiPingEnabled() bool {
//	return s.ApiPingInterval > 0
//}

//func (s *Server) checkMessageEnabled() bool {
//	return s.CheckMessageInterval > 0 && s.TestTeam != "" && s.AliceToken != "" && s.BobToken != ""
//}

func (s *Server) checkCallEnabled() bool {
	return s.CheckCallInterval > 0 && s.TestTeam != "" && s.AliceToken != "" && s.BobToken != ""
}

func (s *Server) String() string {
	return fmt.Sprintf("[%s]", s.Host)
}

type Client struct {
	Name    string
	api     *tdclient.Session
	ws      *tdclient.WsSession
	timeout time.Duration
	contact tdproto.Contact
	token   string
}

func (s *Server) maybeLogin(c *Client, onerror func(error)) error {
	var err error
	if c.api == nil {
		c.api, err = s.tdClient(c.token, c.timeout)
		if err != nil {
			return err
		}
	}

	if c.contact.Jid.Empty() {
		c.contact, err = c.api.Me(s.TestTeam)
		if err != nil {
			return err
		}
		log.Printf("%s check me: %s jid: %s", s, c.Name, c.contact.Jid)
	}

	if c.ws == nil {
		c.ws, err = c.api.Ws(s.TestTeam, onerror)
		if err != nil {
			return err
		}
	}

	return nil
}

//func (s *Server) checkMessage() {
//	if s.checkMessageEnabled() {
//		for {
//			if err := s.doCheckMessage(); err != nil {
//				s.wsFails++
//				s.echoMessageDuration = s.CheckMessageInterval
//				s.checkMessageDuration = s.CheckMessageInterval
//				log.Printf("%s check message: fatal #%d, %s", s, s.wsFails, err)
//				time.Sleep(retryInterval)
//			}
//		}
//	}
//}

//func (s *Server) doCheckMessage() error {
//	errChan := make(chan error)
//	go func() {
//	}()
//
//	return <-errChan
//}

func (s *Server) checkCall() {
	if s.checkCallEnabled() {
		alice := &Client{
			Name:    "alice",
			token:   s.AliceToken,
			timeout: s.CheckCallInterval,
		}
		bob := &Client{
			Name:    "bob",
			token:   s.BobToken,
			timeout: s.CheckCallInterval,
		}
		for {
			if err := s.doCheckCall(alice, bob); err != nil {
				s.callsFails++
				s.checkCallDuration = s.CheckCallInterval
				log.Printf("%s check calls: fatal #%d, %s", s, s.callsFails, err)
				time.Sleep(retryInterval)
			}
		}
	}
}

func (s *Server) webRtcConnect(client *Client, jid *tdproto.JID, iceServer string, name string) (peerConnection *webrtc.PeerConnection, error error) {
	peerConnection, offer, _, err := tdclient.NewPeerConnection(name, iceServer)
	if err != nil {
		return nil, err
	}
	answer, err := tdclient.SendCallOffer(client.ws, client.Name, jid, offer.SDP)
	if err != nil {
		return nil, err
	}

	if err := peerConnection.SetRemoteDescription(answer); err != nil {
		return nil, fmt.Errorf("%s %v: SetRemoteDescription fail: %v", s, name, err)
	}
	return peerConnection, nil
}

func (s *Server) doCheckCall(alice, bob *Client) error {
	errChan := make(chan error)
	onerror := func(err error) { errChan <- err }

	var iceServer string
	for range time.Tick(s.CheckCallInterval) {
		start := time.Now()
		if err := s.maybeLogin(alice, onerror); err != nil {
			return err
		}

		if err := s.maybeLogin(bob, onerror); err != nil {
			return err
		}

		if iceServer == "" {
			features, err := alice.api.Features()
			if err != nil {
				return err
			}
			iceServer = features.ICEServers[0].Urls
		}

		peerConnection, err := s.webRtcConnect(alice, bob.contact.Jid.JID(), iceServer, alice.Name)
		if err != nil {
			return err
		}

		tdclient.SendCallLeave(alice.ws, alice.Name, bob.contact.Jid.JID())
		s.checkCallDuration = time.Since(start)
		log.Printf("%s call test: %s OK", s, s.checkCallDuration.Truncate(time.Millisecond))
		if err := peerConnection.Close(); err != nil {
			return err
		}
	}

	return <-errChan
}
