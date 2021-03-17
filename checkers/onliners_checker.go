package checkers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/tada-team/tdproto"
)

func NewOnlinersChecker() *onlinersChecker {
	p := new(onlinersChecker)
	p.do = p.doCheck
	p.Name = "check_onliners"

	go func() {
		for range time.Tick(time.Second) {
			if !p.lastEvent.IsZero() && time.Since(p.lastEvent) > p.Interval {
				log.Printf("[%s] %s n/a (%s), reset", p.Host, p.Name, time.Since(p.lastEvent).Round(time.Millisecond))
				p.lastEvent = time.Now()
				p.onliners = 0
				p.calls = 0
			}
		}
	}()

	return p
}

type onlinersChecker struct {
	BaseUserChecker

	lastEvent time.Time
	duration  time.Duration
	onliners  int
	calls     int
}

func (p *onlinersChecker) Report(w http.ResponseWriter) {
	if p.Enabled() {
		_, _ = io.WriteString(w, "# TYPE tdcheck_onliners gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_onliners{host=\"%s\"} %d\n", p.Host, p.onliners))
		_, _ = io.WriteString(w, "# TYPE tdcheck_calls gauge\n")
		_, _ = io.WriteString(w, fmt.Sprintf("tdcheck_calls{host=\"%s\"} %d\n", p.Host, p.calls))
	}
}

func (p *onlinersChecker) doCheck() error {
	start := time.Now()

	ev := new(tdproto.ServerOnline)
	if err := p.aliceWsSession.WaitFor(ev); err != nil {
		return err
	}

	p.lastEvent = time.Now()

	if ev.Params.Contacts == nil {
		p.onliners = 0
	} else {
		p.onliners = len(*ev.Params.Contacts)
	}

	if ev.Params.Calls == nil {
		p.calls = 0
	} else {
		p.calls = len(*ev.Params.Calls)
	}

	log.Printf("[%s] %s %s: %d calls: %d", p.Host, p.Name, time.Since(start).Round(time.Millisecond), p.onliners, p.calls)

	return nil
}
