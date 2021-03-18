package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

const maxWsFails = 120

func ServerWatch(s Server, rtr *mux.Router) {
	var wsFails int
	go func() {
		for range time.Tick(time.Minute) {
			if wsFails > maxWsFails {
				log.Panicln("too many ws fails:", wsFails)
			}
			if wsFails > 0 {
				wsFails--
			}
		}
	}()

	var checkers []Checker

	apiPing := NewUrlChecker(s.Host, "tdcheck_api_ping_ms", "/api/v4/ping", s.ApiPingInterval)
	checkers = append(checkers, apiPing)

	nginxPing := NewUrlChecker(s.Host, "tdcheck_nginx_ping_ms", "/ping.txt", s.NginxPingInterval)
	checkers = append(checkers, nginxPing)

	userverPing := NewUrlChecker(s.Host, "tdcheck_userver_ping_ms", s.UServerPingPath, s.UServerPingInterval)
	checkers = append(checkers, userverPing)

	adminPing := NewUrlChecker(s.Host, "tdcheck_admin_ping_ms", "/admin/", s.AdminPingInterval)
	checkers = append(checkers, adminPing)

	wsPing := NewWsPingChecker()
	wsPing.Host = s.Host
	wsPing.Fails = &wsFails
	wsPing.Interval = s.WsPingInterval
	wsPing.Team = s.TestTeam
	wsPing.AliceToken = s.AliceToken
	wsPing.Verbose = s.Verbose
	checkers = append(checkers, wsPing)

	checkOnliners := NewOnlinersChecker()
	checkOnliners.Host = s.Host
	checkOnliners.Fails = &wsFails
	checkOnliners.Interval = s.MaxServerOnlineInterval
	checkOnliners.Team = s.TestTeam
	checkOnliners.AliceToken = s.AliceToken
	checkOnliners.Verbose = s.Verbose
	checkers = append(checkers, checkOnliners)

	checkMessage := NewMessageChecker()
	checkMessage.Host = s.Host
	checkMessage.Fails = &wsFails
	checkMessage.Interval = s.CheckMessageInterval
	checkMessage.Team = s.TestTeam
	checkMessage.AliceToken = s.AliceToken
	checkMessage.BobToken = s.BobToken
	checkMessage.Verbose = s.Verbose
	checkers = append(checkers, checkMessage)

	checkCalls := NewCallsChecker()
	checkCalls.Host = s.Host
	checkCalls.Fails = &wsFails
	checkCalls.Interval = s.CheckCallInterval
	checkCalls.Team = s.TestTeam
	checkCalls.AliceToken = s.AliceToken
	checkCalls.BobToken = s.BobToken
	checkCalls.Verbose = s.Verbose
	checkers = append(checkers, checkCalls)

	path := "/" + s.Host
	log.Println("listen path:", path)

	for _, checker := range checkers {
		if checker.Enabled() {
			go checker.Start()
			log.Println( checker.GetName(), "started")
		} else {
			log.Println(checker.GetName(), "disabled")
		}
	}

	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] request: %s", s.Host, r.Header.Get("User-agent"))

		n := wsFails
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

type Server struct {
	Host string `yaml:"host"`

	TestTeam   string `yaml:"test_team"`
	AliceToken string `yaml:"alice_token"`
	BobToken   string `yaml:"bob_token"`
	Verbose    bool   `yaml:"verbose"`

	ApiPingInterval         time.Duration `yaml:"api_ping_interval"`
	NginxPingInterval       time.Duration `yaml:"nginx_ping_interval"`
	WsPingInterval          time.Duration `yaml:"ws_ping_interval"`
	MaxServerOnlineInterval time.Duration `yaml:"max_server_online_interval"`
	CheckMessageInterval    time.Duration `yaml:"check_message_interval"`
	CheckCallInterval       time.Duration `yaml:"check_call_interval"`
	AdminPingInterval       time.Duration `yaml:"admin_ping_interval"`
	UServerPingInterval     time.Duration `yaml:"userver_ping_interval"`
	UServerPingPath         string        `yaml:"userver_ping_path"`
}
