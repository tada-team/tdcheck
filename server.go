package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
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
}

func (s *Server) Watch(rtr *mux.Router) {
	if s.MaxWsFails == 0 {
		s.MaxWsFails = 10
	}

	var checkers []Checker

	if s.ApiPingInterval > 0 {
		apiPing := NewUrlChecker(s.Host, "tdcheck_api_ping_ms", "/api/v4/ping", s.ApiPingInterval)
		checkers = append(checkers, apiPing)
	}

	if s.NginxPingInterval > 0 {
		nginxPing := NewUrlChecker(s.Host, "tdcheck_nginx_ping_ms", "/ping.txt", s.NginxPingInterval)
		checkers = append(checkers, nginxPing)
	}

	if s.UServerPingInterval > 0 {
		userverPing := NewUrlChecker(s.Host, "tdcheck_userver_ping_ms", s.UServerPingPath, s.UServerPingInterval)
		checkers = append(checkers, userverPing)
	}

	if s.AdminPingInterval > 0 {
		adminPing := NewUrlChecker(s.Host, "tdcheck_admin_ping_ms", "/admin/", s.AdminPingInterval)
		checkers = append(checkers, adminPing)
	}

	aliceSession, err := tdclient.NewSession(ForceScheme(s.Host))
	if err != nil {
		panic(err)
	}
	aliceSession.SetToken(s.AliceToken)
	aliceWebsocket, err := aliceSession.Ws(s.TestTeam)
	if err != nil {
		panic(err)
	}

	bobSession, err := tdclient.NewSession(ForceScheme(s.Host))
	if err != nil {
		panic(err)
	}
	bobSession.SetToken(s.BobToken)
	bobWebsocket, err := aliceSession.Ws(s.TestTeam)
	if err != nil {
		panic(err)
	}

	wsPing := NewWsPingChecker()
	wsPing.Host = s.Host
	wsPing.Fails = s.wsFails
	wsPing.Interval = s.WsPingInterval
	wsPing.Team = s.TestTeam
	wsPing.AliceToken = s.AliceToken
	wsPing.BobToken = s.BobToken
	wsPing.Verbose = s.Verbose

	wsPing.aliceSession = aliceSession
	wsPing.aliceWsSession = aliceWebsocket
	wsPing.bobSession = bobSession
	wsPing.bobWsSession = bobWebsocket

	if wsPing.Enabled() {
		checkers = append(checkers, wsPing)
	}

	checkOnliners := NewOnlinersChecker()
	checkOnliners.Host = s.Host
	checkOnliners.Fails = s.wsFails
	checkOnliners.Interval = s.MaxServerOnlineInterval
	checkOnliners.Team = s.TestTeam
	checkOnliners.AliceToken = s.AliceToken
	checkOnliners.Verbose = s.Verbose

	checkOnliners.aliceSession = aliceSession
	checkOnliners.aliceWsSession = aliceWebsocket
	checkOnliners.bobSession = bobSession
	checkOnliners.bobWsSession = bobWebsocket

	if checkOnliners.Enabled() {
		checkers = append(checkers, checkOnliners)
	}

	checkMessage := NewMessageChecker()
	checkMessage.Host = s.Host
	checkMessage.Fails = s.wsFails
	checkMessage.Interval = s.CheckMessageInterval
	checkMessage.Team = s.TestTeam
	checkMessage.AliceToken = s.AliceToken
	checkMessage.BobToken = s.BobToken
	checkMessage.Verbose = s.Verbose

	checkMessage.aliceSession = aliceSession
	checkMessage.aliceWsSession = aliceWebsocket
	checkMessage.bobSession = bobSession
	checkMessage.bobWsSession = bobWebsocket

	if checkMessage.Enabled() {
		checkers = append(checkers, checkMessage)
	}

	path := "/" + s.Host
	log.Println("listen path:", path)

	go checkersDispatch(checkers)

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] request: %s", s.Host, r.Header.Get("User-agent"))

		n := s.wsFails
		//if n == 1 { // XXX:
		//	n = 0
		//}

		_, _ = io.WriteString(w, "# TYPE tdcheck_ws_fails gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_fails{host=\"%s\"} %d\n", s.Host, n))

		for _, checker := range checkers {
			checker.Report(w)
		}
	})
}

func checkersDispatch(checkers []Checker) {
	selectCases := make([]reflect.SelectCase, len(checkers))

	for i, checker := range checkers {
		interval := checker.GetInveral()
		newTicker := time.NewTicker(interval)

		selectCases[i] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(newTicker.C),
		}
	}

	for {
		chosenIndex, _, _ := reflect.Select(selectCases)

		err := checkers[chosenIndex].DoCheck()
		if err != nil {
			panic(err)
		}
	}
}

func (s *Server) wsFailsHarakiri() {
	for range time.Tick(5 * time.Second) {
		if s.wsFails > s.MaxWsFails {
			log.Panicln("too many ws fails:", s.wsFails, ">", s.MaxWsFails)
		}
		if s.wsFails > 0 {
			s.wsFails--
			log.Println("decrease fails to", s.wsFails)
		}
	}
}
