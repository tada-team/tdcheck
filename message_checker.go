package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/tada-team/kozma"
	"github.com/tada-team/tdclient"
	"github.com/tada-team/tdproto"
)

func NewMessageChecker() *messageChecker {
	p := new(messageChecker)
	p.do = p.doCheck
	p.Name = "message_checker"
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

func (p *messageChecker) Report(w io.Writer) {
	if p.Enabled() {
		_, _ = io.WriteString(w, "# TYPE tdcheck_echo_message_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_echo_message_ms{host=\"%s\"} %d\n", p.Host, p.echoMessageDuration.Milliseconds()))

		_, _ = io.WriteString(w, "# TYPE tdcheck_check_message_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_check_message_ms{host=\"%s\"} %d\n", p.Host, p.checkMessageDuration.Milliseconds()))
	}
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
	log.Printf("[%s] %s: alice send `%s` (uid: %s)", p.Host, p.Name, text, messageId)

	for time.Since(start) < p.Interval {
		msg, delayed, err := p.aliceWsSession.WaitForMessage()
		if err == tdclient.Timeout {
			log.Printf("[%s] %s: alice got timeout on `%s`", p.Host, p.Name, text)
			p.numTimeouts++
			if p.numTimeouts > p.maxTimeouts {
				p.echoMessageDuration = p.Interval
				p.checkMessageDuration = p.Interval
				return err
			}
			break
		} else if err != nil {
			p.echoMessageDuration = p.Interval
			p.checkMessageDuration = p.Interval
			return err
		}

		if !delayed || !msg.Chat.JID().Equal(p.bobJid) {
			continue
		}

		log.Printf("[%s] %s: alice got `%s` (gentime: %v)", p.Host, p.Name, msg.PushText, msg.Gentime)
		if msg.MessageId == messageId {
			log.Printf("[%s] %s: echo ok (%s)", p.Host, p.Name, p.echoMessageDuration.Round(time.Millisecond))
			p.echoMessageDuration = time.Since(start)
			break
		}
	}

	if p.echoMessageDuration == p.Interval{
		p.checkMessageDuration = p.Interval
		return nil
	}

	for time.Since(start) < p.Interval {
		msg, delayed, err := p.bobWsSession.WaitForMessage()
		if err == tdclient.Timeout {
			log.Printf("[%s] %s: bob got timeout on `%s`", p.Host, p.Name, text)
			p.numTimeouts++
			if p.numTimeouts > p.maxTimeouts {
				p.checkMessageDuration = p.Interval
				return err
			}
			break
		} else if err != nil {
			p.checkMessageDuration = p.Interval
			return err
		}

		if delayed {
			continue
		}

		log.Printf("[%s] %s: bob got `%s` (gentime: %v)", p.Host, p.Name, msg.PushText, msg.Gentime)
		if msg.MessageId == messageId {
			log.Printf("[%s] %s: delivery ok (%s)", p.Host, p.Name, p.checkMessageDuration.Round(time.Millisecond))
			p.checkMessageDuration = time.Since(start)
			break
		}
	}

	log.Printf("[%s] %s: alice drop `%s`", p.Host, p.Name, text)
	p.aliceWsSession.DeleteMessage(messageId)

	return nil
}
