package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/tada-team/kozma"
	"github.com/tada-team/tdclient"
)

const (
	retryInterval = time.Second
	wsFailsCheck  = time.Minute
	maxTimeouts   = 10
	maxWsFails    = 120
)

var maxTimeoutsReached = errors.New("max timouts")

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

	CheckMessageInterval time.Duration `yaml:"check_message_interval"`
	echoMessageDuration  time.Duration
	checkMessageDuration time.Duration
	wsFails              int
}

func (s Server) tdClient(token string, timeout time.Duration) tdclient.Session {
	if !strings.HasPrefix(s.Host, "http") {
		s.Host = "https://" + s.Host
	}
	sess, err := tdclient.NewSession(s.Host)
	if err != nil {
		log.Panicln(err)
	}
	sess.Timeout = timeout
	sess.SetToken(token)
	sess.SetVerbose(s.Verbose)
	return sess
}

func (s Server) apiPingEnabled() bool {
	return s.ApiPingInterval > 0
}

func (s Server) userverPingEnabled() bool {
	return s.UserverPingInterval > 0 && s.UserverPingPath != ""
}

func (s Server) wsPingEnabled() bool {
	return s.WsPingInterval > 0 && s.TestTeam != "" && s.AliceToken != ""
}

func (s Server) checkMessageEnabled() bool {
	return s.CheckMessageInterval > 0 && s.TestTeam != "" && s.AliceToken != "" && s.BobToken != ""
}

func (s Server) String() string {
	return fmt.Sprintf("[%s]", s.Host)
}

func (s Server) Watch(rtr *mux.Router) {
	go s.apiPing()
	go s.userverPing()
	go s.wsPing()
	go s.checkMessage()
	go s.paniker()

	path := "/" + s.Host
	log.Println(
		"listen path:", path,
		"api ping:", s.apiPingEnabled(),
		"ws ping:", s.wsPingEnabled(),
		"userver:", s.userverPingEnabled(),
		"message:", s.checkMessageEnabled(),
	)

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s request", s)

		if s.apiPingEnabled() {
			io.WriteString(w, "# TYPE tdcheck_api_ping_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_api_ping_ms{host=\"%s\"} %d\n", s.Host, s.apiPingDuration.Milliseconds()))
		}

		if s.userverPingEnabled() {
			io.WriteString(w, "# TYPE tdcheck_userver_ping_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_userver_ping_ms{host=\"%s\"} %d\n", s.Host, s.userverPingDuration.Milliseconds()))
		}

		if s.wsPingEnabled() {
			io.WriteString(w, "# TYPE tdcheck_ws_ping_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_ws_ping_ms{host=\"%s\"} %d\n", s.Host, s.wsPingDuration.Milliseconds()))
		}

		if s.checkMessageEnabled() {
			io.WriteString(w, "# TYPE tdcheck_echo_message_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_echo_message_ms{host=\"%s\"} %d\n", s.Host, s.echoMessageDuration.Milliseconds()))

			io.WriteString(w, "# TYPE tdcheck_check_message_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_check_message_ms{host=\"%s\"} %d\n", s.Host, s.checkMessageDuration.Milliseconds()))

			io.WriteString(w, "# TYPE tdcheck_ws_fails gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_ws_fails{host=\"%s\"} %d\n", s.Host, s.wsFails))
		}
	})
}

func (s *Server) apiPing() {
	if !s.apiPingEnabled() {
		return
	}

	interval := s.ApiPingInterval
	client := s.tdClient(s.BobToken, interval)

	for range time.Tick(interval) {
		start := time.Now()
		err := client.Ping()
		s.apiPingDuration = time.Since(start)

		if err != nil {
			log.Printf("%s ping: %s fail: %s", s, s.apiPingDuration.Truncate(time.Millisecond), err)
			s.apiPingDuration = interval
			continue
		}

		log.Printf("%s ping: %s OK", s, s.apiPingDuration.Truncate(time.Millisecond))
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

		if s.Verbose {
			log.Printf("%s userver content of %s (%d byte(s))", s, s.UserverPingPath, len(content))
		}

		if err != nil {
			log.Printf("%s userver ping: %s fail: %s", s, s.userverPingDuration.Truncate(time.Millisecond), err)
			s.userverPingDuration = interval
			continue
		}

		log.Printf("%s ping: %s OK", s, s.userverPingDuration.Truncate(time.Millisecond))
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
				log.Printf("%s check: fatal #%d, %s", s, s.wsFails, err)
				time.Sleep(retryInterval)
			}
		}
	}
}

func (s *Server) doCheckMessage() error {
	errChan := make(chan error)

	interval := s.CheckMessageInterval
	aliceClient := s.tdClient(s.AliceToken, interval)
	alice, err := aliceClient.Me(s.TestTeam)
	if err != nil {
		return err
	}
	log.Printf("%s check: alice jid: %s", s, alice.Jid)

	aliceWs, err := aliceClient.Ws(s.TestTeam, func(err error) { errChan <- err })
	if err != nil {
		return err
	}

	bobClient := s.tdClient(s.BobToken, interval)
	bob, err := bobClient.Me(s.TestTeam)
	if err != nil {
		return err
	}
	log.Printf("%s check: bob jid: %s", s, bob.Jid)

	bobWs, err := bobClient.Ws(s.TestTeam, func(err error) { errChan <- err })
	if err != nil {
		return err
	}

	numTimouts := 0
	go func() {
		for range time.Tick(interval) {
			start := time.Now()

			text := kozma.Say()
			messageId := aliceWs.SendPlainMessage(bob.Jid, text)
			log.Printf("%s check: alice send %s: %s", s, messageId, text)

			for time.Since(start) < interval {
				msg, delayed, err := aliceWs.WaitForMessage()
				s.echoMessageDuration = time.Since(start)
				if err == tdclient.Timeout {
					log.Printf("%s check: alice got timeout on %s", s, messageId)
					numTimouts++
					if numTimouts > maxTimeouts {
						errChan <- err
						return
					}
					break
				}
				if err != nil {
					errChan <- err
					return
				}
				if !delayed {
					continue
				}
				log.Printf("%s check: alice got %s", s, msg.MessageId)
				if msg.MessageId == messageId {
					log.Printf("%s check: echo %s OK", s, s.echoMessageDuration.Truncate(time.Millisecond))
					break
				}
			}

			for time.Since(start) < interval {
				msg, delayed, err := bobWs.WaitForMessage()
				s.checkMessageDuration = time.Since(start)
				if err == tdclient.Timeout {
					log.Printf("%s check: bob got timeout on %s", s, messageId)
					numTimouts++
					if numTimouts > maxTimeouts {
						errChan <- maxTimeoutsReached
						return
					}
					break
				}
				if err != nil {
					errChan <- err
					return
				}
				if delayed {
					continue
				}
				log.Printf("%s check: bob got %s: %s", s, msg.MessageId, msg.PushText)
				if msg.MessageId == messageId {
					log.Printf("%s check: delivery %s OK", s, s.checkMessageDuration.Truncate(time.Millisecond))
					break
				}
			}

			log.Printf("%s check: alice drop %s", s, messageId)
			aliceWs.DeleteMessage(messageId)
		}
	}()

	return <-errChan
}

func (s *Server) wsPing() {
	if s.wsPingEnabled() {
		for {
			if err := s.doWsPing(); err != nil {
				s.wsFails++
				log.Printf("%s ws ping: fatal #%d, %s", s, s.wsFails, err)
				time.Sleep(retryInterval)
			}
		}
	}
}

func (s *Server) doWsPing() error {
	errChan := make(chan error)

	interval := s.WsPingInterval
	aliceClient := s.tdClient(s.AliceToken, interval)
	aliceWs, err := aliceClient.Ws(s.TestTeam, func(err error) { errChan <- err })
	if err != nil {
		return err
	}

	numTimouts := 0
	go func() {
		for range time.Tick(interval) {
			start := time.Now()
			uid := aliceWs.Ping()
			log.Printf("%s ws ping: alice send ping %s", s, uid)
			for time.Since(start) < interval {
				confirmId, err := aliceWs.WaitForConfirm()
				s.wsPingDuration = time.Since(start)
				if err == tdclient.Timeout {
					log.Printf("%s ws ping: alice got ping timeout on %s", s, uid)
					numTimouts++
					if numTimouts > maxTimeouts {
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

func (s *Server) paniker() {
	for range time.Tick(wsFailsCheck) {
		if s.wsFails > maxWsFails {
			log.Panicln("too many ws fails:", s.wsFails)
		}
		if s.wsFails > 0 {
			s.wsFails--
		}
	}
}
