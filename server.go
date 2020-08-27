package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/tada-team/kozma"
)

type Server struct {
	Host string `yaml:"host"`

	ApiPingInterval time.Duration `yaml:"api_ping_interval"`
	apiPingDuration time.Duration

	CheckMessageInterval time.Duration `yaml:"check_message_interval"`
	CheckMessageTeam     string        `yaml:"check_message_team"`
	echoMessageDuration  time.Duration
	checkMessageDuration time.Duration
	wsFails              int

	AliceToken string `yaml:"alice_token"`
	BobToken   string `yaml:"bob_token"`
	Verbose    bool   `yaml:"verbose"`
}

func (s Server) apiPingEnabled() bool {
	return s.ApiPingInterval > 0
}

func (s Server) checkMessageEnabled() bool {
	return s.CheckMessageInterval > 0 && s.CheckMessageTeam != "" && s.AliceToken != "" && s.BobToken != ""
}

func (s Server) String() string {
	return fmt.Sprintf("[%s]", s.Host)
}

func (s Server) Watch(rtr *mux.Router) {
	go s.ping()
	go s.checkMessage()

	path := "/" + s.Host
	log.Println("watch:", path)

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if s.apiPingEnabled() {
			io.WriteString(w, "# TYPE tdcheck_api_ping_ms gauge\n")
			io.WriteString(w, fmt.Sprintf("tdcheck_api_ping_ms{host=\"%s\"} %d\n", s.Host, s.apiPingDuration.Milliseconds()))
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
	client := TdClient{
		Host:    s.Host,
		Timeout: interval,
		Verbose: s.Verbose,
	}

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
			err := s.doCheckMessage()
			if err != nil {
				log.Println("check: fatal:", err)
				s.wsFails++
			}
		}
	}
}
func (s *Server) doCheckMessage() error {
	interval := s.CheckMessageInterval

	aliceClient := TdClient{
		Host:    s.Host,
		Token:   s.AliceToken,
		Verbose: s.Verbose,
		Timeout: interval,
	}
	aliceJid, err := aliceClient.MyJID(s.CheckMessageTeam)
	if err != nil {
		return err
	}
	log.Println("check: alice jid:", aliceJid)

	aliceWs, err := aliceClient.WsClient(s.CheckMessageTeam, nil)
	if err != nil {
		return err
	}

	bobClient := TdClient{
		Host:    s.Host,
		Token:   s.BobToken,
		Verbose: s.Verbose,
		Timeout: interval,
	}
	bobJid, err := bobClient.MyJID(s.CheckMessageTeam)
	if err != nil {
		return err
	}
	log.Println("check: bob jid:", bobJid)

	bobWs, err := bobClient.WsClient(s.CheckMessageTeam, nil)
	if err != nil {
		return err
	}

	for range time.Tick(interval) {
		start := time.Now()

		text := kozma.Say()
		messageId := aliceWs.sendPlainMessage(bobJid, text)
		log.Printf("%s check: alice send %s: %s", s, messageId, text)

		for {
			msg, delayed, err := aliceWs.waitForMessage(interval)
			if err == wsTimeout {
				log.Printf("%s check: alice got timeout on %s", s, messageId)
				s.echoMessageDuration = interval
				break
			}
			if err != nil {
				return err
			}
			if !delayed {
				continue
			}
			log.Printf("%s check: alice got %s", s, msg.MessageId)
			s.echoMessageDuration = time.Since(start)
			if msg.MessageId == messageId {
				log.Printf("%s check: echo %s OK", s, s.echoMessageDuration.Truncate(time.Millisecond))
				break
			}
		}

		for {
			msg, delayed, err := bobWs.waitForMessage(interval)
			if err == wsTimeout {
				log.Printf("%s check: bob got timeout on %s", s, messageId)
				s.checkMessageDuration = interval
				break
			}
			if err != nil {
				return err
			}
			if delayed {
				continue
			}
			log.Printf("%s check: bob got %s: %s", s, msg.MessageId, msg.PushText)
			s.checkMessageDuration = time.Since(start)
			if msg.MessageId == messageId {
				log.Printf("%s check: echo %s OK", s, s.checkMessageDuration.Truncate(time.Millisecond))
				break
			}
		}

		log.Printf("%s check: alice drop %s", s, messageId)
		aliceWs.deleteMessage(messageId)
	}
	return nil
}
