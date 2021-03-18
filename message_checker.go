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
	return p
}

type messageChecker struct {
	BaseUserChecker

	echoMessageDuration  time.Duration
	checkMessageDuration time.Duration

	bobJid tdproto.JID
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
	p.echoMessageDuration = 0
	p.checkMessageDuration = 0
	defer func() {
		if p.echoMessageDuration == 0 {
			p.echoMessageDuration = p.Interval
			p.checkMessageDuration = p.Interval
		} else if p.checkMessageDuration == 0 {
			p.checkMessageDuration = p.Interval
		}
	}()

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

	for time.Since(start) < p.Interval && p.aliceWsSession != nil {
		msg, delayed, err := p.aliceWsSession.WaitForMessage()
		if err == tdclient.Timeout {
			log.Printf("[%s] %s: alice got timeout on `%s`", p.Host, p.Name, text)
			continue
		} else if err != nil {
			return err
		}

		if !delayed || !msg.Chat.JID().Equal(p.bobJid) || msg.MessageId != messageId {
			log.Printf("[%s] %s: alice skip echo `%s`", p.Host, p.Name, msg.PushText)
			continue
		}

		log.Printf("[%s] %s: alice got echo `%s` (%s)", p.Host, p.Name, msg.PushText, p.echoMessageDuration.Round(time.Millisecond))
		p.echoMessageDuration = time.Since(start)
		break
	}

	if p.echoMessageDuration > 0 {
		for time.Since(start) < p.Interval && p.bobWsSession != nil {
			msg, delayed, err := p.bobWsSession.WaitForMessage()
			if err == tdclient.Timeout {
				log.Printf("[%s] %s: bob got timeout on `%s`", p.Host, p.Name, text)
				continue
			} else if err != nil {
				return err
			}

			if delayed || msg.MessageId != messageId {
				log.Printf("[%s] %s: bob skip `%s`", p.Host, p.Name, msg.PushText)
				continue
			}

			p.checkMessageDuration = time.Since(start)
			log.Printf("[%s] %s: bob got `%s` (%s)", p.Host, p.Name, msg.PushText, p.checkMessageDuration.Round(time.Millisecond))

			break
		}
	}

	log.Printf("[%s] %s: alice drop `%s`", p.Host, p.Name, text)
	p.aliceWsSession.DeleteMessage(messageId)

	return nil
}
