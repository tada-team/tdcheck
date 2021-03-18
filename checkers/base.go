package checkers

import (
	"io"
	"log"
	"strings"
	"time"

	"github.com/tada-team/tdclient"
)

const (
	retryInterval = time.Second
	maxTimeouts   = 10
)

type Checker interface {
	Enabled() bool
	Start()
	Report(w io.Writer)
}

func ForceScheme(url string) string {
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}
	return url
}

type BaseUserChecker struct {
	Host       string
	Name       string
	Interval   time.Duration
	Team       string
	AliceToken string
	BobToken   string
	Verbose    bool
	Fails      *int

	aliceSession   *tdclient.Session
	aliceWsSession *tdclient.WsSession

	bobSession   *tdclient.Session
	bobWsSession *tdclient.WsSession

	do func() error // hack for inheritance
}

func (p *BaseUserChecker) Enabled() bool { return p.Interval > 0 && p.Team != "" && p.AliceToken != "" }

func (p *BaseUserChecker) Start() {
	if p.do == nil {
		panic("do() not implemented")
	}

	if !p.Enabled() {
		return
	}

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	onfail := func(err error) {
		if err == nil {
			return
		}
		*p.Fails++
		log.Printf("[%s] %s: fatal #%d, %s", p.Host, p.Name, *p.Fails, err)
		time.Sleep(retryInterval)
	}

	var err error
	for {
		if p.AliceToken != "" && (p.aliceSession == nil || p.aliceWsSession == nil) {
			p.aliceSession, p.aliceWsSession, err = p.auth(p.AliceToken, func(err error) {
				onfail(err)
				log.Printf("[%s] %s: reset alice session", p.Host, p.Name)
				p.aliceSession, p.aliceWsSession = nil, nil
			})
			if err != nil {
				onfail(err)
				continue
			}
		}

		if p.BobToken != "" && (p.bobSession == nil || p.bobWsSession == nil) {
			p.bobSession, p.bobWsSession, err = p.auth(p.BobToken, onfail)
			if err != nil {
				onfail(err)
				log.Printf("[%s] %s: reset bob session", p.Host, p.Name)
				p.bobSession, p.bobWsSession = nil, nil
				continue
			}
		}

		if err := p.do(); err != nil {
			onfail(err)
		}

		<-ticker.C
	}
}

func (p *BaseUserChecker) auth(token string, onfail func(error)) (*tdclient.Session, *tdclient.WsSession, error) {
	session, err := tdclient.NewSession(ForceScheme(p.Host))
	if err != nil {
		return nil, nil, err
	}

	session.Timeout = p.Interval
	session.SetVerbose(p.Verbose)
	session.SetToken(token)

	wsSession, err := session.Ws(p.Team, onfail)
	if err != nil {
		return nil, nil, err
	}

	return &session, wsSession, err
}
