package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type TdClient struct {
	Host     string
	Insecure bool
	Timeout  time.Duration
	Token    string
	Verbose  bool
}

func (c *TdClient) WsClient(team string, onfail func(error)) (*wsClient, error) {
	if c.Token == "" {
		log.Panicln("empty token")
	}

	path := strings.Replace(c.url(fmt.Sprintf("/messaging/%s", team)), "http", "ws", 1)
	conn, _, err := websocket.DefaultDialer.Dial(path, http.Header{
		"token": []string{c.Token},
	})

	if err != nil {
		return nil, err
	}

	w := &wsClient{
		TdClient: c,
		team:     team,
		conn:     conn,
		inbox:    make(chan event, 100),
		outbox:   make(chan event, 100),
		fail:     make(chan error),
	}

	go func() {
		err := <-w.fail
		if err != nil {
			if onfail == nil {
				log.Panic("ws client fail:", err)
			}
			onfail(err)
		}
	}()

	go w.outboxLoop()
	go w.inboxLoop()

	return w, nil
}

func (c TdClient) Ping() error {
	resp := new(struct {
		Ok     bool   `json:"ok"`
		Result string `json:"result"`
	})
	_, err := c.doGet("/api/v4/ping", resp)
	return err
}

type me struct {
	Jid string `json:"jid"`
}

type team struct {
	Me me `json:"me"`
}

func (c TdClient) MyJID(teamUid string) (string, error) {
	resp := new(struct {
		Ok     bool   `json:"ok"`
		Result team   `json:"result"`
		Error  string `json:"error"`
	})

	b, err := c.doGet("/api/v4/teams/"+teamUid, resp)
	if err != nil {
		return "", err
	}

	if err := json.Unmarshal(b, resp); err != nil {
		return "", errors.Wrap(err, "unmarshall fail")
	}

	if !resp.Ok {
		return "", errors.New(resp.Error)
	}

	return resp.Result.Me.Jid, nil
}

func (c TdClient) httpClient() *http.Client {
	return &http.Client{
		Timeout: c.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func (c TdClient) url(path string) string {
	protocol := "http"
	if !c.Insecure {
		protocol += "s"
	}
	return protocol + "://" + c.Host + path
}

func (c TdClient) doGet(path string, v interface{}) ([]byte, error) {
	client := c.httpClient()

	req, err := http.NewRequest("GET", c.url(path), nil)
	if err != nil {
		return []byte{}, errors.Wrap(err, "new request fail")
	}

	if c.Token != "" {
		req.Header.Set("token", c.Token)
	}

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
