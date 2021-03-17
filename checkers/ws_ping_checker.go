package checkers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func NewWsPingChecker() *wsPingChecker {
	p := new(wsPingChecker)
	p.do = p.doPingCheck
	return p
}

type wsPingChecker struct {
	BaseUserChecker
	duration time.Duration
}

func (p *wsPingChecker) doPingCheck() error {
	start := time.Now()

	uid := p.aliceWsSession.Ping()
	log.Printf("[%s] %s: send %s", p.Host, p.Name, uid)
	confirmId, err := p.aliceWsSession.WaitForConfirm()
	if err != nil {
		p.duration = p.Interval
		return err
	}

	p.duration = time.Since(start)
	if confirmId == uid {
		log.Printf("[%s] %s: got in %s", p.Host, p.Name, p.duration.Round(time.Millisecond))
	}

	return nil
}

func (p *wsPingChecker) Report(w http.ResponseWriter) {
	if p.Enabled() {
		_, _ = io.WriteString(w, fmt.Sprintf("# TYPE %s gauge\n", p.Name))
		_, _ = io.WriteString(w, fmt.Sprintf("%s{host=\"%s\"} %d\n", p.Name, p.Host, p.duration.Milliseconds()))
	}
}
