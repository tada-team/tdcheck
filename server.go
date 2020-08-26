package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

type Server struct {
	Host            string        `yaml:"host"`
	Token           string        `yaml:"key"`
	ApiPingInterval time.Duration `yaml:"api_ping_interval"`
	apipingDuration time.Duration
}

func (s Server) String() string { return s.Host }

func (s Server) Watch(rtr *mux.Router) {
	go s.ping()
	path := "/" + s.Host
	log.Println("watch:", path)
	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "# TYPE tdcheck_api_ping_ms gauge\n")
		io.WriteString(w, fmt.Sprintf("tdcheck_api_ping_ms{host=\"%s\"} %d\n", s.Host, s.apipingDuration.Milliseconds()))
	})
}

func (s *Server) ping() {
	interval := 5 * time.Second
	for range time.Tick(interval) {
		start := time.Now()

		client := TdClient{Host: s.Host, Timeout: interval}

		err := client.Ping()
		s.apipingDuration = time.Since(start)

		if err != nil {
			log.Printf("%s ping: %s fail: %s", s, s.apipingDuration.Truncate(time.Millisecond), err)
			s.apipingDuration = interval
			continue
		}

		log.Printf("%s ping: %s OK", s, s.apipingDuration.Truncate(time.Millisecond))
	}
}
