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

	err := p.aliceWsSession.ForeachMessage(func(c1 chan tdproto.Message, c2 chan error) {
		text := kozma.Say()
		start := time.Now()
		messageId := p.aliceWsSession.SendPlainMessage(p.bobJid, text)
		log.Printf("[%s] %s: alice send `%s` (uid: %s)", p.Host, p.Name, text, messageId)
		timeoutTimer := time.After(time.Second * 10)

		for {
			select {
			case m, ok := <-c1:
				if !ok {
					c2 <- fmt.Errorf("channel closed")
					return
				}
				if m.Content.Text != text {
					continue
				}

				currentEchoMessageDuration = time.Since(start)
				log.Printf("[%s] %s: alice got echo `%s` (%dms)", p.Host, p.Name, m.PushText, roundMilliseconds(currentEchoMessageDuration))
				c2 <- nil
				return
			case <-timeoutTimer:
				c2 <- tdclient.Timeout
				return
			}
		}
	})
	currentCheckMessageDuration = currentEchoMessageDuration
	if err != nil {
		return err
	}

	return nil
}
