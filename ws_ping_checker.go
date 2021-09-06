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
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_ping_ms{host=\"%s\"} %d\n", p.Host, roundMilliseconds(p.duration)))
	}
}

func (p *wsPingChecker) doCheck() error {
	p.duration = 0
	defer func() {
		if p.duration == 0 {
			p.duration = p.Interval
		}
	}()

	start := time.Now()
	v := p.aliceWsSession.Ping()
	log.Printf("[%s] %s: send %s", p.Host, p.Name, v)

	numTimeouts := 0
	for time.Since(start) < p.Interval {
		if p.aliceWsSession == nil { // reset
			return nil
		}

		confirmId, err := p.aliceWsSession.WaitForConfirm()
		if err == tdclient.Timeout {
			numTimeouts++
			log.Printf("[%s] %s: timeout #%d %s (%s)", p.Host, p.Name, numTimeouts, v, time.Since(start).Round(time.Millisecond))
			continue
		} else if err != nil {
			log.Printf("[%s] %s: fail %s (%s)", p.Host, p.Name, v, time.Since(start).Round(time.Millisecond))
			return err
		}

		if confirmId != v {
			log.Printf("[%s] %s: invalid %s (%s)", p.Host, p.Name, v, time.Since(start).Round(time.Millisecond))
			continue
		}

		p.duration = time.Since(start)
		log.Printf("[%s] %s: got %s (%s)", p.Host, p.Name, v, p.duration.Round(time.Millisecond))

		break
	}

	return nil
}
