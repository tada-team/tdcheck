package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/tada-team/tdclient"
)

type Server struct {
	Host string `yaml:"host"`

	TestTeam   string `yaml:"test_team"`
	AliceToken string `yaml:"alice_token"`
	BobToken   string `yaml:"bob_token"`
	Verbose    bool   `yaml:"verbose"`
	MaxWsFails int    `yaml:"max_ws_fails"`

	ApiPingInterval         time.Duration `yaml:"api_ping_interval"`
	NginxPingInterval       time.Duration `yaml:"nginx_ping_interval"`
	WsPingInterval          time.Duration `yaml:"ws_ping_interval"`
	MaxServerOnlineInterval time.Duration `yaml:"max_server_online_interval"`
	CheckMessageInterval    time.Duration `yaml:"check_message_interval"`
	CheckCallInterval       time.Duration `yaml:"check_call_interval"`
	AdminPingInterval       time.Duration `yaml:"admin_ping_interval"`
	UServerPingInterval     time.Duration `yaml:"userver_ping_interval"`
	UServerPingPath         string        `yaml:"userver_ping_path"`

	wsFails int

	lastCheckersRun time.Time

	aliceSession   *tdclient.Session
	aliceWsSession *tdclient.WsSession

	bobSession   *tdclient.Session
	bobWsSession *tdclient.WsSession

	checkers []Checker
}

var defaultUpdateTime time.Duration = time.Second * 10

func (s *Server) Watch(rtr *mux.Router) {
	if s.MaxWsFails == 0 {
		s.MaxWsFails = 10
	}

	s.checkers = make([]Checker, 0)

	if s.ApiPingInterval > 0 {
		apiPing := NewUrlChecker("tdcheck_api_ping_ms", "/api/v4/ping", s.ApiPingInterval, s)
		s.checkers = append(s.checkers, apiPing)
	}

	if s.NginxPingInterval > 0 {
		nginxPing := NewUrlChecker("tdcheck_nginx_ping_ms", "/ping.txt", s.NginxPingInterval, s)
		s.checkers = append(s.checkers, nginxPing)
	}

	if s.UServerPingInterval > 0 {
		userverPing := NewUrlChecker("tdcheck_userver_ping_ms", s.UServerPingPath, s.UServerPingInterval, s)
		s.checkers = append(s.checkers, userverPing)
	}

	if s.AdminPingInterval > 0 {
		adminPing := NewUrlChecker("tdcheck_admin_ping_ms", "/admin/", s.AdminPingInterval, s)
		s.checkers = append(s.checkers, adminPing)
	}

	aliceSession, err := tdclient.NewSession(ForceScheme(s.Host))
	if err != nil {
		log.Fatalf("Failed to create alice session: %q", (err))
	}
	aliceSession.SetToken(s.AliceToken)
	aliceWebsocket, err := aliceSession.Ws(s.TestTeam)
	if err != nil {
		log.Fatalf("Failed to create alice websocket: %q", (err))
	}

	bobSession, err := tdclient.NewSession(ForceScheme(s.Host))
	if err != nil {
		log.Fatalf("Failed to create bob session: %q", (err))
	}
	bobSession.SetToken(s.BobToken)
	bobWebsocket, err := aliceSession.Ws(s.TestTeam)
	if err != nil {
		log.Fatalf("Failed to create bob websocket: %q", (err))
	}

	s.aliceSession = aliceSession
	s.aliceWsSession = aliceWebsocket
	s.bobSession = bobSession
	s.bobWsSession = bobWebsocket

	wsPing := NewWsPingChecker(s)
	wsPing.Interval = s.WsPingInterval
	wsPing.Team = s.TestTeam

	if wsPing.Enabled() {
		s.checkers = append(s.checkers, wsPing)
	}

	checkOnliners := NewOnlinersChecker(s)
	checkOnliners.Interval = s.MaxServerOnlineInterval
	checkOnliners.Team = s.TestTeam

	if checkOnliners.Enabled() {
		s.checkers = append(s.checkers, checkOnliners)
	}

	checkMessage := NewMessageChecker(s)
	checkMessage.Interval = s.CheckMessageInterval
	checkMessage.Team = s.TestTeam

	if checkMessage.Enabled() {
		s.checkers = append(s.checkers, checkMessage)
	}

	path := "/" + s.Host
	log.Println("listen path:", path)

	s.aliceWsSession.Close()
	s.bobWsSession.Close()
	s.lastCheckersRun = time.Now()
	err = s.runCheckers()
	if err != nil {
		log.Fatalf("Failed to run initial checkers: %q", (err))
	}

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] request: %s", s.Host, r.Header.Get("User-agent"))

		if defaultUpdateTime < time.Since(s.lastCheckersRun) {
			s.lastCheckersRun = time.Now()
			err := s.runCheckers()
			if err != nil {
				log.Fatalf("Failed to run checker: %q", (err))
			}
		}

		n := s.wsFails
		//if n == 1 { // XXX:
		//	n = 0
		//}

		_, _ = io.WriteString(w, "# TYPE tdcheck_ws_fails gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_fails{host=\"%s\"} %d\n", s.Host, n))

		for _, checker := range s.checkers {
			checker.Report(w)
		}
	})
}

func (s *Server) runCheckers() error {
	// TODO: Check for errors
	s.aliceWsSession.Start()
	s.bobWsSession.Start()

	for _, checker := range s.checkers {
		err := checker.DoCheck()
		if err != nil {
			log.Printf("Checker %s failed: %q\n", checker.GetName(), err)
		}
	}

	s.aliceWsSession.Close()
	s.bobWsSession.Close()

	return nil
}
