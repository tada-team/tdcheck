package main

import (
	"encoding/json"
	"log"
	"time"

	uuid "github.com/satori/go.uuid"

	"github.com/pkg/errors"

	"github.com/gorilla/websocket"
)

type params map[string]interface{}

type event struct {
	Name   string `json:"event"`
	Params params `json:"params"`
	raw    []byte
}

type wsClient struct {
	*TdClient
	team   string
	conn   *websocket.Conn
	closed bool
	inbox  chan event
	outbox chan event
	fail   chan error
}

func (w *wsClient) sendPlainMessage(to, text string) string {
	type messageContent struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}

	uid := uuid.NewV4().String()
	w.send("client.message.updated", params{
		"message_id": uid,
		"to":         to,
		"content": messageContent{
			Type: "plain",
			Text: text,
		},
	})

	return uid
}

func (w *wsClient) deleteMessage(uid string) {
	w.send("client.message.delete", params{
		"message_id": uid,
	})
}

var wsTimeout = errors.New("Timeout")

type message struct {
	PushText  string `json:"push_text,omitempty"`
	MessageId string `json:"message_id"`
}

type serverMessageUpdated struct {
	Name   string `json:"event"`
	Params struct {
		Messages []message `json:"messages"`
	} `json:"params"`
}

func (w *wsClient) waitForMessage(timeout time.Duration) (message, error) {
	v := serverMessageUpdated{}
	err := w.waitFor("server.message.updated", timeout, &v)
	if err != nil {
		return message{}, err
	}
	return v.Params.Messages[0], nil
}

func (w *wsClient) waitFor(name string, timeout time.Duration, v interface{}) error {
	for {
		select {
		case event := <-w.inbox:
			if w.Verbose {
				log.Println("got:", string(event.raw))
			}
			if event.Name == name {
				if err := json.Unmarshal(event.raw, &v); err != nil {
					w.fail <- errors.Wrap(err, "json fail")
					return nil
				}
				return nil
			}
		case <-time.After(timeout):
			return wsTimeout
		}
	}
}

func (w *wsClient) send(name string, params params) {
	w.outbox <- event{
		Name:   name,
		Params: params,
	}
}

func (w *wsClient) outboxLoop() {
	for !w.closed {
		data := <-w.outbox

		b, err := json.Marshal(data)
		if err != nil {
			w.fail <- errors.Wrap(err, "json marshal fail")
			return
		}

		if w.Verbose {
			log.Println("send:", string(b))
		}

		if err := w.conn.WriteMessage(websocket.BinaryMessage, b); err != nil {
			w.fail <- errors.Wrap(err, "test client fail")
			return
		}
	}
}

func (w wsClient) inboxLoop() {
	for !w.closed {
		_, data, err := w.conn.ReadMessage()
		if err != nil {
			w.fail <- errors.Wrap(err, "conn read fail")
			return
		}

		v := event{}
		if err := json.Unmarshal(data, &v); err != nil {
			w.fail <- errors.Wrap(err, "json fail")
			return
		}

		v.raw = data
		w.inbox <- v
	}
}
