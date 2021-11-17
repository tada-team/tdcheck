package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/tada-team/tdclient"
)

func NewWsPingChecker(parentServer *Server) *wsPingChecker {
	p := new(wsPingChecker)
	p.Name = "ws_ping_checker"
	p.parentServer = parentServer
	return p
}

type wsPingChecker struct {
	BaseUserChecker
	duration time.Duration
}

func (p *wsPingChecker) Report(w io.Writer) {
	if p.Enabled() {
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		_, _ = io.WriteString(w, "# TYPE tdcheck_ws_ping_ms gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_ws_ping_ms{host=\"%s\"} %d\n", p.parentServer.Host, roundMilliseconds(p.duration)))
	}
}

func (p *wsPingChecker) DoCheck() error {
	var currentDuration time.Duration = 0
	defer func() {
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		p.duration = currentDuration
	}()

	start := time.Now()
	v := p.parentServer.aliceWsSession.Ping()
	log.Printf("[%s] %s: send %s", p.parentServer.Host, p.Name, v)

	numTimeouts := 0

	if p.parentServer.aliceWsSession == nil { // reset
		return nil
	}

	confirmId, err := p.parentServer.aliceWsSession.WaitForConfirm()
	if err == tdclient.Timeout {
		numTimeouts++
		log.Printf("[%s] %s: timeout #%d %s (%s)", p.parentServer.Host, p.Name, numTimeouts, v, time.Since(start).Round(time.Millisecond))

	} else if err != nil {
		log.Printf("[%s] %s: fail %s (%s)", p.parentServer.Host, p.Name, v, time.Since(start).Round(time.Millisecond))
		return err
	}

	if confirmId != v {
		log.Printf("[%s] %s: invalid %s (%s)", p.parentServer.Host, p.Name, v, time.Since(start).Round(time.Millisecond))
	}

	currentDuration = time.Since(start)
	log.Printf("[%s] %s: got %s (%dms)", p.parentServer.Host, p.Name, v, roundMilliseconds(currentDuration))

	return nil
}
