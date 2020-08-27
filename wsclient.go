package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type params map[string]interface{}

type event struct {
	Name      string `json:"event"`
	Params    params `json:"params"`
	ConfirmId string `json:"confirm_id,omitempty"`
	raw       []byte
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

	uid := uuid.New().String()

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

func (w *wsClient) ping() string {
	return w.send("client.ping", params{})
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
		Delayed  bool      `json:"delayed"`
	} `json:"params"`
}

type serverConfirm struct {
	Name   string `json:"event"`
	Params struct {
		ConfirmId string `json:"confirm_id"`
	} `json:"params"`
}

func (w *wsClient) waitForMessage(timeout time.Duration) (message, bool, error) {
	v := serverMessageUpdated{}
	err := w.waitFor("server.message.updated", timeout, &v)
	if err != nil {
		return message{}, false, err
	}
	return v.Params.Messages[0], v.Params.Delayed, nil
}

func (w *wsClient) waitForConfirm(timeout time.Duration) (string, error) {
	v := serverConfirm{}
	err := w.waitFor("server.confirm", timeout, &v)
	if err != nil {
		return "", err
	}
	return v.Params.ConfirmId, nil
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

func (w *wsClient) send(name string, params params) string {
	uid := uuid.New().String()
	w.outbox <- event{
		Name:      name,
		Params:    params,
		ConfirmId: uid,
	}
	return uid
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

		if v.ConfirmId != "" {
			w.send("client.confirm", params{
				"confirm_id": v.ConfirmId,
			})
		}

		v.raw = data
		w.inbox <- v
	}
}
