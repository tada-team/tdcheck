package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/tada-team/tdproto"
)

func NewOnlinersChecker(parentServer *Server) *onlinersChecker {
	p := new(onlinersChecker)
	p.Name = "onliners_checker"
	p.parentServer = parentServer
	return p
}

type onlinersChecker struct {
	BaseUserChecker

	lastEvent time.Time
	onliners  int
	calls     int
}

func (p *onlinersChecker) Report(w io.Writer) {
	if p.Enabled() {
		_, _ = io.WriteString(w, "# TYPE tdcheck_onliners gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_onliners{host=\"%s\"} %d\n", p.parentServer.Host, p.onliners))
		_, _ = io.WriteString(w, "# TYPE tdcheck_calls gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls{host=\"%s\"} %d\n", p.parentServer.Host, p.calls))
	}
}

func (p *onlinersChecker) DoCheck() error {
	var currentOnliners int = 0
	var currentCalls int = 0

	defer func() {
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		p.onliners = currentOnliners
		p.calls = currentCalls
	}()

	start := time.Now()

	ev := new(tdproto.ServerOnline)
	err := p.parentServer.aliceWsSession.WaitFor(ev)
	if err != nil {
		return err
	}

	p.lastEvent = time.Now()

	if ev.Params.Contacts == nil {
		currentOnliners = 0
	} else {
		currentOnliners = len(ev.Params.Contacts)
	}

	if ev.Params.Calls == nil {
		currentCalls = 0
	} else {
		currentCalls = len(ev.Params.Calls)
	}

	log.Printf("[%s] %s %s: %d calls: %d", p.parentServer.Host, p.Name, time.Since(start).Round(time.Millisecond), currentOnliners, currentCalls)

	return nil
}
