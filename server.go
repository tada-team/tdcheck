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
	Host         string `yaml:"host"`
	Token        string `yaml:"key"`
	pingDuration time.Duration
}

func (s Server) String() string { return s.Host }

func (s Server) Watch(rtr *mux.Router) {
	go s.ping()
	path := "/" + s.Host + ".txt"
	log.Println("watch:", path)
	rtr.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "# TYPE api_ping_ms gauge\n")
		io.WriteString(w, fmt.Sprintf("api_ping_ms %d\n", s.pingDuration.Milliseconds()))
	})
}

func (s *Server) ping() {
	interval := 5 * time.Second
	for range time.Tick(interval) {
		start := time.Now()

		client := TdClient{Host: s.Host, Timeout: interval}

		err := client.Ping()
		s.pingDuration = time.Since(start)

		if err != nil {
			log.Printf("%s ping: %s fail: %s", s, s.pingDuration.Truncate(time.Millisecond), err)
			s.pingDuration = interval
			continue
		}

		log.Printf("%s ping: %s OK", s, s.pingDuration.Truncate(time.Millisecond))
	}
}
