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
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		_, _ = io.WriteString(w, "# TYPE tdcheck_echo_message_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_echo_message_ms{host=\"%s\"} %d\n", p.Host, roundMilliseconds(p.echoMessageDuration)))

		_, _ = io.WriteString(w, "# TYPE tdcheck_check_message_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_check_message_ms{host=\"%s\"} %d\n", p.Host, roundMilliseconds(p.checkMessageDuration)))
	}
}

func (p *messageChecker) DoCheck() error {
	var currentEchoMessageDuration time.Duration = 0
	var currentCheckMessageDuration time.Duration = 0
	defer func() {
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		p.echoMessageDuration = currentEchoMessageDuration
		p.checkMessageDuration = currentCheckMessageDuration
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

	numTimeouts := 0

	msg, delayed, err := p.aliceWsSession.WaitForMessage()
	if err == tdclient.Timeout {
		numTimeouts++
		log.Printf("[%s] %s: alice timeout #%d on `%s`", p.Host, p.Name, numTimeouts, text)
	} else if err != nil {
		return err
	}

	if !delayed || !(msg.Chat.JID().String() == p.bobJid.String()) || msg.MessageId != messageId {
		log.Printf("[%s] %s: alice skip echo `%s`", p.Host, p.Name, msg.PushText)
	}

	currentEchoMessageDuration = time.Since(start)
	log.Printf("[%s] %s: alice got echo `%s` (%dms)", p.Host, p.Name, msg.PushText, roundMilliseconds(currentEchoMessageDuration))

	if currentEchoMessageDuration > 0 {
		numTimeouts := 0
		for time.Since(start) < p.Interval && p.bobWsSession != nil {
			msg, delayed, err := p.bobWsSession.WaitForMessage()
			if err == tdclient.Timeout {
				numTimeouts++
				log.Printf("[%s] %s: bob timeout #%d on `%s`", p.Host, p.Name, numTimeouts, text)
				continue
			} else if err != nil {
				return err
			}

			if delayed || msg.MessageId != messageId {
				log.Printf("[%s] %s: bob skip `%s`", p.Host, p.Name, msg.PushText)
				continue
			}

			currentCheckMessageDuration = time.Since(start)
			log.Printf("[%s] %s: bob got `%s` (%dms)", p.Host, p.Name, msg.PushText, roundMilliseconds(currentCheckMessageDuration))

			break
		}
	}

	if p.aliceWsSession != nil {
		log.Printf("[%s] %s: alice drop `%s`", p.Host, p.Name, text)
		p.aliceWsSession.DeleteMessage(messageId)
	}

	return nil
}
