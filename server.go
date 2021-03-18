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

	var adminPing checkers.UrlChecker
	adminPing.Host = s.Host
	adminPing.Name = "tdcheck_admin_ping_ms"
	adminPing.Path = "/admin/"
	adminPing.Interval = s.AdminPingInterval
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
