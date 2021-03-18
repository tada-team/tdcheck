package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/tada-team/tdclient"
)

func NewWsPingChecker() *wsPingChecker {
	p := new(wsPingChecker)
	p.do = p.doCheck
	p.Name = "ws_ping_checker"
	return p
}

type wsPingChecker struct {
	BaseUserChecker
	duration time.Duration
}

func (p *wsPingChecker) Report(w io.Writer) {
	if p.Enabled() {
		_, _ = io.WriteString(w, "# TYPE tdcheck_ws_ping_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_ping_ms{host=\"%s\"} %d\n", p.Host, p.duration.Milliseconds()))
	}
}

func (p *wsPingChecker) doCheck() error {
	start := time.Now()

	uid := p.aliceWsSession.Ping()
	log.Printf("[%s] %s: send %s", p.Host, p.Name, uid)
	p.duration = p.Interval

	for time.Since(start) < p.Interval && p.aliceWsSession != nil {
		confirmId, err := p.aliceWsSession.WaitForConfirm()
		if err == tdclient.Timeout {
			continue
		} else if err != nil {
			return err
		}
		if confirmId == uid {
			p.duration = time.Since(start)
			log.Printf("[%s] %s: got in %s", p.Host, p.Name, p.duration.Round(time.Millisecond))
		}
		break
	}

	return nil
}
