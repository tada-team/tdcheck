package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gorilla/mux"
	"github.com/pion/webrtc/v2"
	"github.com/pkg/errors"
	"github.com/tada-team/kozma"
	"github.com/tada-team/tdclient"
	"github.com/tada-team/tdproto"
)

const (
	retryInterval = time.Second
	wsFailsCheck  = time.Minute
	maxTimeouts   = 10
	maxWsFails    = 120
)

var maxTimeoutsReached = errors.New("max timeouts")

func ServerWatch(s Server, rtr *mux.Router) {
	go s.apiPing()
	go s.userverPing()
	go s.wsPing()
	go s.checkMessage()
	go s.checkOnliners()
	go s.checkCall()
	go s.panickier()

	path := "/" + s.Host
	log.Println(
		"listen path:", path,
		"|",
		"api ping:", s.apiPingEnabled(),
		"|",
		"ws ping:", s.wsPingEnabled(),
		"|",
		"userver:", s.userverPingEnabled(),
		"|",
		"message:", s.checkMessageEnabled(),
		"|",
		"calls:", s.checkCallEnabled(),
		"|",
		"onliners:", s.checkOnlinersEnabled(),
	)

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] request", s.Host)

		if s.apiPingEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_api_ping_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_api_ping_ms{host=\"%s\"} %d\n", s.Host, s.apiPingDuration.Milliseconds()))
		}

		if s.userverPingEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_userver_ping_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_userver_ping_ms{host=\"%s\"} %d\n", s.Host, s.userverPingDuration.Milliseconds()))
		}

		if s.wsPingEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_ws_ping_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_ping_ms{host=\"%s\"} %d\n", s.Host, s.wsPingDuration.Milliseconds()))
		}

		if s.checkOnlinersEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_onliners gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_onliners{host=\"%s\"} %d\n", s.Host, s.onliners))
			_, _ = io.WriteString(w, "# TYPE tdcheck_calls gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls{host=\"%s\"} %d\n", s.Host, s.calls))
		}

		if s.checkMessageEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_echo_message_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_echo_message_ms{host=\"%s\"} %d\n", s.Host, s.echoMessageDuration.Milliseconds()))

			_, _ = io.WriteString(w, "# TYPE tdcheck_check_message_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_check_message_ms{host=\"%s\"} %d\n", s.Host, s.checkMessageDuration.Milliseconds()))

			_, _ = io.WriteString(w, "# TYPE tdcheck_ws_fails gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_fails{host=\"%s\"} %d\n", s.Host, s.wsFails))
		}

		if s.checkCallEnabled() {
			_, _ = io.WriteString(w, "# TYPE tdcheck_calls_fails counter\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls_fails{host=\"%s\"} %d\n", s.Host, s.callsFails))
			_, _ = io.WriteString(w, "# TYPE tdcheck_calls_ms gauge\n")
			_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls_ms{host=\"%s\"} %d\n", s.Host, s.checkCallDuration.Milliseconds()))
		}
	})
}

type Server struct {
	Host string `yaml:"host"`

	TestTeam   string `yaml:"test_team"`
	AliceToken string `yaml:"alice_token"`
	BobToken   string `yaml:"bob_token"`
	Verbose    bool   `yaml:"verbose"`

	ApiPingInterval time.Duration `yaml:"api_ping_interval"`
	apiPingDuration time.Duration

	WsPingInterval time.Duration `yaml:"ws_ping_interval"`
	wsPingDuration time.Duration

	UserverPingInterval time.Duration `yaml:"userver_ping_interval"`
	UserverPingPath     string        `yaml:"userver_ping_path"`
	userverPingDuration time.Duration

	MaxServerOnlineInterval time.Duration `yaml:"max_server_online_interval"`
	onliners                int
	calls                   int

	CheckMessageInterval time.Duration `yaml:"check_message_interval"`
	CheckCallInterval    time.Duration `yaml:"check_call_interval"`
	echoMessageDuration  time.Duration
	checkMessageDuration time.Duration
	checkCallDuration    time.Duration
	wsFails              int
	callsFails           int
}

func (s *Server) tdClient(token string, timeout time.Duration) (*tdclient.Session, error) {
	url := s.Host
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	sess, err := tdclient.NewSession(url)
	if err != nil {
		return nil, err
	}

	sess.Timeout = timeout
	sess.SetToken(token)
	sess.SetVerbose(s.Verbose)
	return &sess, nil
}

func (s *Server) apiPingEnabled() bool {
	return s.ApiPingInterval > 0
}

func (s *Server) checkOnlinersEnabled() bool {
	return s.MaxServerOnlineInterval > 0
}

func (s *Server) userverPingEnabled() bool {
	return s.UserverPingInterval > 0 && s.UserverPingPath != ""
}

func (s *Server) wsPingEnabled() bool {
	return s.WsPingInterval > 0 && s.TestTeam != "" && s.AliceToken != ""
}

func (s *Server) checkMessageEnabled() bool {
	return s.CheckMessageInterval > 0 && s.TestTeam != "" && s.AliceToken != "" && s.BobToken != ""
}

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

func (s *Server) apiPing() {
	if !s.apiPingEnabled() {
		return
	}

	var err error
	var client *tdclient.Session

	interval := s.ApiPingInterval
	for range time.Tick(interval) {
		if client == nil {
			client, err = s.tdClient(s.BobToken, interval)
			if err != nil {
				log.Printf("%s api ping: connect error: %s", s, err)
				s.apiPingDuration = interval
				continue
			}
		}

		start := time.Now()

		err := client.Ping()
		s.apiPingDuration = time.Since(start)

		if err != nil {
			log.Printf("%s api ping: %s fail: %s", s, s.apiPingDuration.Truncate(time.Millisecond), err)
			s.apiPingDuration = interval
			continue
		}

		log.Printf("%s api ping: %s OK", s, s.apiPingDuration.Truncate(time.Millisecond))
	}
}

func (s *Server) userverPing() {
	if !s.userverPingEnabled() {
		return
	}

	interval := s.UserverPingInterval
	for range time.Tick(interval) {
		start := time.Now()
		content, err := s.checkContent(s.UserverPingPath)
		s.userverPingDuration = time.Since(start)

		if err != nil || len(content) == 0 {
			log.Printf(
				"%s userver ping: %s fail: %v",
				s,
				s.userverPingDuration.Truncate(time.Millisecond),
				err,
			)
			s.userverPingDuration = interval
			continue
		}

		log.Printf(
			"%s userver ping: %s %s OK: %s",
			s,
			s.UserverPingPath,
			s.userverPingDuration.Truncate(time.Millisecond),
			humanize.Bytes(uint64(len(content))),
		)
	}
}

func (s *Server) checkContent(path string) ([]byte, error) {
	if strings.HasPrefix(s.Host, "http") {
		path = s.Host + path
	} else {
		path = "https://" + s.Host + path
	}

	req, err := http.NewRequest("GET", path, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request fail")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "client do fail")
	}
	defer resp.Body.Close()

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body fail")
	}

	if resp.StatusCode != 200 {
		return nil, errors.Wrapf(err, "status code: %d %s", resp.StatusCode, string(respData))
	}

	return respData, nil
}

func (s *Server) checkMessage() {
	if s.checkMessageEnabled() {
		for {
			if err := s.doCheckMessage(); err != nil {
				s.wsFails++
				s.echoMessageDuration = s.CheckMessageInterval
				s.checkMessageDuration = s.CheckMessageInterval
				log.Printf("%s check message: fatal #%d, %s", s, s.wsFails, err)
				time.Sleep(retryInterval)
			}
		}
	}
}

func (s *Server) checkOnliners() {
	if !s.checkOnlinersEnabled() {
		return
	}

	lastServerOnline := time.Now()

	go func() {
		for range time.Tick(time.Second) {
			if time.Since(lastServerOnline) > s.MaxServerOnlineInterval {
				log.Printf("%s server online n/a (%s), reset", s, time.Since(lastServerOnline).Truncate(time.Millisecond))
				lastServerOnline = time.Now()
				s.onliners = 0
				s.calls = 0
			}
		}
	}()

	client := &Client{
		Name:    "alice check onliners",
		token:   s.AliceToken,
		timeout: s.MaxServerOnlineInterval,
	}

	for {
		if err := s.maybeLogin(client, func(err error) {
			s.wsFails++
			log.Printf("%s ws fail #%d (maybe login), %s", s, s.wsFails, err)
			time.Sleep(retryInterval)
		}); err != nil {
			s.wsFails++
			log.Printf("%s ws fail #%d, %s", s, s.wsFails, err)
			time.Sleep(retryInterval)
			continue
		}

		start := time.Now()

		ev := new(tdproto.ServerOnline)
		if err := client.ws.WaitFor(ev); err != nil {
			continue
		}

		lastServerOnline = time.Now()

		if ev.Params.Contacts == nil {
			s.onliners = 0
		} else {
			s.onliners = len(*ev.Params.Contacts)
		}

		if ev.Params.Calls == nil {
			s.calls = 0
		} else {
			s.calls = len(*ev.Params.Calls)
		}

		log.Printf("%s got server online in %s: %d calls: %d", s, time.Since(start).Truncate(time.Millisecond), s.onliners, s.calls)
	}
}

func (s *Server) doCheckMessage() error {
	errChan := make(chan error)
	go func() {
		interval := s.CheckMessageInterval
		alice := &Client{
			Name:    "alice",
			token:   s.AliceToken,
			timeout: interval,
		}

		bob := &Client{
			Name:    "bob",
			token:   s.BobToken,
			timeout: interval,
		}

		numTimeouts := 0
		for range time.Tick(interval) {
			if err := s.maybeLogin(alice, func(err error) { errChan <- err }); err != nil {
				errChan <- err
				return
			}

			if err := s.maybeLogin(bob, func(err error) { errChan <- err }); err != nil {
				errChan <- err
				return
			}

			start := time.Now()
			text := kozma.Say()
			messageId := alice.ws.SendPlainMessage(bob.contact.Jid, text)
			log.Printf("%s check message: alice send %s: %s", s, messageId, text)

			for time.Since(start) < interval {
				msg, delayed, err := alice.ws.WaitForMessage()
				s.echoMessageDuration = time.Since(start)
				s.checkMessageDuration = interval
				if err == tdclient.Timeout {
					log.Printf("%s check message: alice got timeout on %s", s, messageId)
					numTimeouts++
					if numTimeouts > maxTimeouts {
						errChan <- err
						return
					}
					break
				}
				if err != nil {
					errChan <- err
					return
				}
				if delayed {
					log.Printf("%s check message: alice got %s", s, msg.MessageId)
					if msg.MessageId == messageId {
						log.Printf("%s check message: echo %s OK", s, s.echoMessageDuration.Truncate(time.Millisecond))
						break
					}
				}
			}

			for time.Since(start) < interval {
				msg, delayed, err := bob.ws.WaitForMessage()
				s.checkMessageDuration = time.Since(start)
				if err == tdclient.Timeout {
					log.Printf("%s check message: bob got timeout on %s", s, messageId)
					numTimeouts++
					if numTimeouts > maxTimeouts {
						errChan <- maxTimeoutsReached
						return
					}
					break
				}
				if err != nil {
					errChan <- err
					return
				}
				if !delayed {
					log.Printf("%s check message: bob got %s: %s", s, msg.MessageId, msg.PushText)
					if msg.MessageId == messageId {
						log.Printf("%s check message: delivery %s OK", s, s.checkMessageDuration.Truncate(time.Millisecond))
						break
					}
				}
			}

			log.Printf("%s check message: alice drop %s", s, messageId)
			alice.ws.DeleteMessage(messageId)
		}
	}()

	return <-errChan
}

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
				log.Printf("%s check calls: fatal #%d, %s", s, s.wsFails, err)
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

	if err := s.maybeLogin(alice, onerror); err != nil {
		return err
	}

	if err := s.maybeLogin(bob, onerror); err != nil {
		return err
	}

	features, err := alice.api.Features()
	if err != nil {
		return err
	}

	iceServer := features.ICEServers[0].Urls

	for range time.Tick(s.CheckCallInterval) {
		start := time.Now()

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

func (s *Server) wsPing() {
	if s.wsPingEnabled() {
		for {
			if err := s.doWsPing(); err != nil {
				s.wsFails++
				s.wsPingDuration = s.WsPingInterval
				log.Printf("%s ws ping: fatal #%d, %s", s, s.wsFails, err)
				time.Sleep(retryInterval)
			}
		}
	}
}

func (s *Server) doWsPing() error {
	errChan := make(chan error)

	numTimeouts := 0
	go func() {
		var err error
		var aliceClient *tdclient.Session
		var aliceWs *tdclient.WsSession

		interval := s.WsPingInterval
		for range time.Tick(interval) {
			if aliceClient == nil {
				aliceClient, err = s.tdClient(s.AliceToken, interval)
				if err != nil {
					errChan <- err
					return
				}
			}

			if aliceWs == nil {
				aliceWs, err = aliceClient.Ws(s.TestTeam, func(err error) { errChan <- err })
				if err != nil {
					errChan <- err
					return
				}
			}

			start := time.Now()
			uid := aliceWs.Ping()
			log.Printf("%s ws ping: alice send ping %s", s, uid)
			for time.Since(start) < interval {
				confirmId, err := aliceWs.WaitForConfirm()
				s.wsPingDuration = time.Since(start)
				if err == tdclient.Timeout {
					log.Printf("%s ws ping: alice got ping timeout on %s", s, uid)
					numTimeouts++
					if numTimeouts > maxTimeouts {
						errChan <- maxTimeoutsReached
						return
					}
					break
				}
				if err != nil {
					errChan <- err
					return
				}
				if confirmId == uid {
					log.Printf("%s ws ping: %s OK", s, s.wsPingDuration.Truncate(time.Millisecond))
					break
				}
			}
		}
	}()

	return <-errChan
}

func (s *Server) panickier() {
	for range time.Tick(wsFailsCheck) {
		if s.wsFails > maxWsFails {
			log.Panicln("too many ws fails:", s.wsFails)
		}
		if s.wsFails > 0 {
			s.wsFails--
		}
	}
}
