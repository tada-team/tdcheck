package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/gorilla/mux"
	"github.com/tada-team/kozma"
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

	CheckMessageInterval time.Duration `yaml:"check_message_interval"`
	echoMessageDuration  time.Duration
	checkMessageDuration time.Duration
	wsFails              int
}

func (s Server) tdClient(token string, timeout time.Duration) TdClient {
	return TdClient{
		Host:    s.Host,
		Verbose: s.Verbose,
		Token:   token,
		Timeout: timeout,
	}
}

func (s Server) apiPingEnabled() bool {
	return s.ApiPingInterval > 0
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
	go s.ping()
	go s.wsPing()
	go s.checkMessage()
	go s.paniker()

	path := "/" + s.Host
	log.Println("watch:", path)

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s request", s)
		if s.apiPingEnabled() {
			io.WriteString(w, "# TYPE tdcheck_api_ping_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_api_ping_ms{host=\"%s\"} %d\n", s.Host, s.apiPingDuration.Milliseconds()))
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

func (s *Server) ping() {
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
	aliceJid, err := aliceClient.MyJID(s.TestTeam)
	if err != nil {
		return err
	}
	log.Printf("%s check: alice jid: %s", s, aliceJid)

	aliceWs, err := aliceClient.WsClient(s.TestTeam, func(err error) { errChan <- err })
	if err != nil {
		return err
	}

	bobClient := s.tdClient(s.BobToken, interval)
	bobJid, err := bobClient.MyJID(s.TestTeam)
	if err != nil {
		return err
	}
	log.Printf("%s check: bob jid: %s", s, bobJid)

	bobWs, err := bobClient.WsClient(s.TestTeam, func(err error) { errChan <- err })
	if err != nil {
		return err
	}

	numTimouts := 0
	go func() {
		for range time.Tick(interval) {
			start := time.Now()

			text := kozma.Say()
			messageId := aliceWs.sendPlainMessage(bobJid, text)
			log.Printf("%s check: alice send %s: %s", s, messageId, text)

			for time.Since(start) < interval {
				msg, delayed, err := aliceWs.waitForMessage(interval)
				s.echoMessageDuration = time.Since(start)
				if err == wsTimeout {
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
				msg, delayed, err := bobWs.waitForMessage(interval)
				s.checkMessageDuration = time.Since(start)
				if err == wsTimeout {
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
			aliceWs.deleteMessage(messageId)
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
	aliceWs, err := aliceClient.WsClient(s.TestTeam, func(err error) { errChan <- err })
	if err != nil {
		return err
	}

	numTimouts := 0
	go func() {
		for range time.Tick(interval) {
			start := time.Now()
			uid := aliceWs.ping()
			log.Printf("%s ws ping: alice send ping %s", s, uid)
			for time.Since(start) < interval {
				confirmId, err := aliceWs.waitForConfirm(interval)
				s.wsPingDuration = time.Since(start)
				if err == wsTimeout {
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
