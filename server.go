package tdcheck

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type Server struct {
	Host         string `yaml:"host"`
	Token        string `yaml:"key"`
	pingDuration time.Duration
}

func (s Server) String() string         { return s.Host }
func (s Server) url(path string) string { return "https://" + s.Host + path }

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

		resp := new(struct {
			Ok     bool   `json:"ok"`
			Result string `json:"result"`
		})

		_, err := s.doGet(interval, resp)
		s.pingDuration = time.Since(start)

		if err != nil {
			log.Printf("%s ping: %s fail: %s", s, s.pingDuration.Truncate(time.Millisecond), err)
			s.pingDuration = interval
			continue
		}

		log.Printf("%s ping: %s OK", s, s.pingDuration.Truncate(time.Millisecond))
	}
}

func (s *Server) doGet(timeout time.Duration, v interface{}) ([]byte, error) {
	var client = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	req, err := http.NewRequest("GET", s.url("/api/v4/ping"), nil)
	if err != nil {
		return []byte{}, errors.Wrap(err, "new request fail")
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, errors.Wrap(err, "client do fail")
	}
	defer resp.Body.Close()

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return respData, errors.Wrap(err, "read body fail")
	}

	if resp.StatusCode != 200 {
		return respData, errors.Wrapf(err, "status code: %d %s", resp.StatusCode, string(respData))
	}

	if err := json.Unmarshal(respData, &v); err != nil {
		return respData, errors.Wrapf(err, "unmarshal fail on: %s", string(respData))
	}

	return respData, nil
}
