package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/tada-team/tdcheck/checkers"
)

const (
	wsFailsCheck = time.Minute
	maxWsFails   = 120
)

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

	apiPing := checkers.NewUrlChecker(s.Host, "tdcheck_api_ping_ms", "/api/v4/ping", s.ApiPingInterval)
	go apiPing.Start()

	nginxPing := checkers.NewUrlChecker(s.Host, "tdcheck_nginx_ping_ms", "/ping.txt", s.NginxPingInterval)
	go nginxPing.Start()

	userverPing := checkers.NewUrlChecker(s.Host, "tdcheck_userver_ping_ms", s.UServerPingPath, s.UServerPingInterval)
	go userverPing.Start()

	adminPing := checkers.NewUrlChecker(s.Host, "tdcheck_admin_ping_ms", "/admin/", s.AdminPingInterval)
	go adminPing.Start()

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

	checkCalls := checkers.NewCallsChecker()
	checkCalls.Host = s.Host
	checkCalls.Fails = &wsFails
	checkCalls.Interval = s.CheckCallInterval
	checkCalls.Team = s.TestTeam
	checkCalls.AliceToken = s.AliceToken
	checkCalls.BobToken = s.BobToken
	checkCalls.Verbose = s.Verbose
	go checkCalls.Start()

	path := "/" + s.Host
	log.Println(
		"listen path:", path,
		"| api:", apiPing.Enabled(),
		"| nginx:", nginxPing.Enabled(),
		"| userver:", userverPing.Enabled(),
		"| admin:", adminPing.Enabled(),
		"| ws ping:", wsPing.Enabled(),
		"| message:", checkCalls.Enabled(),
		"| calls:", checkCalls.Enabled(),
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
		checkCalls.Report(w)

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
