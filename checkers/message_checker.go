package checkers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/tada-team/kozma"
	"github.com/tada-team/tdclient"
	"github.com/tada-team/tdproto"
)

func NewMessageChecker() *messageChecker {
	p := new(messageChecker)
	p.do = p.doCheck
	p.Name = "tdcheck_message"
	p.maxTimeouts = 100
	return p
}

type messageChecker struct {
	BaseUserChecker

	echoMessageDuration  time.Duration
	checkMessageDuration time.Duration

	bobJid tdproto.JID

	maxTimeouts int
	numTimeouts int
}

func (p *messageChecker) doCheck() error {
	if p.bobJid.Empty() {
		contact, err := p.bobSession.Me(p.Team)
		if err != nil {
			return err
		}
		p.bobJid = contact.Jid
	}

	start := time.Now()
	text := kozma.Say()
	messageId := p.aliceWsSession.SendPlainMessage(p.bobJid, text)
	log.Printf("[%s] %s: alice send %s: %s", p.Host, p.Name, messageId, text)

	for time.Since(start) < p.Interval {
		msg, delayed, err := p.aliceWsSession.WaitForMessage()
		p.echoMessageDuration = time.Since(start)
		p.checkMessageDuration = p.Interval
		if err == tdclient.Timeout {
			log.Printf("[%s] %s: alice got timeout on %s", p.Host, p.Name, messageId)
			p.numTimeouts++
			if p.numTimeouts > p.maxTimeouts {
				return err
			}
			break
		}
		if err != nil {
			return err
		}
		if delayed {
			log.Printf("[%s] %s: alice got %s", p.Host, p.Name, msg.MessageId)
			if msg.MessageId == messageId {
				log.Printf("[%s] %s: echo %s OK", p.Host, p.Name, p.echoMessageDuration.Round(time.Millisecond))
				break
			}
		}
	}

	for time.Since(start) < p.Interval {
		msg, delayed, err := p.bobWsSession.WaitForMessage()
		p.checkMessageDuration = time.Since(start)
		if err == tdclient.Timeout {
			log.Printf("[%s] %s: bob got timeout on %s", p.Host, p.Name, messageId)
			p.numTimeouts++
			if p.numTimeouts > p.maxTimeouts {
				return err
			}
			break
		}
		if err != nil {
			return err
		}
		if !delayed {
			log.Printf("[%s] %s: bob got %s: %s", p.Host, p.Name, msg.MessageId, msg.PushText)
			if msg.MessageId == messageId {
				log.Printf("[%s] %s: delivery %s OK", p.Host, p.Name, p.checkMessageDuration.Round(time.Millisecond))
				break
			}
		}
	}

	log.Printf("[%s] %s: alice drop %s", p.Host, p.Name, messageId)
	p.aliceWsSession.DeleteMessage(messageId)

	return nil
}

func (p *messageChecker) Report(w http.ResponseWriter) {
	if p.Enabled() {
		_, _ = io.WriteString(w, "# TYPE tdcheck_echo_message_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_echo_message_ms{host=\"%s\"} %d\n", p.Host, p.echoMessageDuration.Milliseconds()))

		_, _ = io.WriteString(w, "# TYPE tdcheck_check_message_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_check_message_ms{host=\"%s\"} %d\n", p.Host, p.checkMessageDuration.Milliseconds()))
	}
}
