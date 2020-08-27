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

	AliceToken string `yaml:"alice_token"`
	BobToken   string `yaml:"bob_token"`
}

func (s Server) apiPingEnabled() bool {
	return s.apiPingDuration > 0
}

func (s Server) checkMessageEnabled() bool {
	return s.CheckMessageInterval > 0 && s.CheckMessageTeam != "" && s.AliceToken != "" && s.BobToken != ""
}

func (s Server) String() string { return s.Host }

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
		}
	})
}

func (s *Server) ping() {
	if !s.apiPingEnabled() {
		return
	}

	interval := s.ApiPingInterval
	client := TdClient{Host: s.Host, Timeout: interval}

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
	if !s.checkMessageEnabled() {
		return
	}

	interval := s.CheckMessageInterval

	aliceClient := TdClient{Host: s.Host, Timeout: interval, Token: s.AliceToken}
	aliceJid, err := aliceClient.MyJID(s.CheckMessageTeam)
	if err != nil {
		log.Panicln(err)
	}
	log.Println("check: alice jid:", aliceJid)

	aliceWs, err := aliceClient.WsClient(s.CheckMessageTeam, nil)
	if err != nil {
		log.Panicln(err)
	}

	bobClient := TdClient{Host: s.Host, Timeout: interval, Token: s.BobToken}
	bobJid, err := bobClient.MyJID(s.CheckMessageTeam)
	if err != nil {
		log.Panicln(err)
	}
	log.Println("check: bob jid:", bobJid)

	bobWs, err := bobClient.WsClient(s.CheckMessageTeam, nil)
	if err != nil {
		log.Panicln(err)
	}

	for range time.Tick(interval) {
		start := time.Now()

		text := kozma.Say()
		messageId := aliceWs.sendPlainMessage(bobJid, text)
		log.Printf("%s check: alice send %s: %s", s, messageId, text)

		for {
			msg, err := aliceWs.waitForMessage(interval)
			if err != nil {
				log.Panicln(err)
			}
			log.Printf("%s check: alice got %s", s, messageId)
			s.echoMessageDuration = time.Since(start)
			if msg.MessageId == messageId {
				break
			}
		}

		for {
			msg, err := bobWs.waitForMessage(interval)
			if err != nil {
				log.Panicln(err)
			}
			log.Printf("%s check: bob got %s: %s", s, msg.MessageId, msg.PushText)
			s.checkMessageDuration = time.Since(start)
			if msg.MessageId == messageId {
				break
			}
		}

		log.Printf("%s check: alice drop %s", s, messageId)
		aliceWs.deleteMessage(messageId)
	}
}
