package main

import (
	"io"
	"strings"
	"sync"
	"time"

	"github.com/tada-team/tdclient"
)

type Checker interface {
	Enabled() bool
	DoCheck() error
	GetName() string
	GetInveral() time.Duration
	Report(w io.Writer)
}

func ForceScheme(url string) string {
	if !strings.HasPrefix(url, "http") {
		return "https://" + url
	}
	return url
}

type BaseUserChecker struct {
	Host     string
	Name     string
	Interval time.Duration
	Team     string
	//Verbose  bool
	Fails int

	aliceSession   *tdclient.Session
	aliceWsSession *tdclient.WsSession

	bobSession   *tdclient.Session
	bobWsSession *tdclient.WsSession

	updateDurationMutex sync.Mutex
}

func (p *BaseUserChecker) GetName() string { return p.Name }

func (p *BaseUserChecker) Enabled() bool { return p.Interval > 0 }

func DoCheck() error {
	panic("Base checker is called")
}

func (p *BaseUserChecker) GetInveral() time.Duration {
	return p.Interval
}

func roundMilliseconds(theDuration time.Duration) int64 {
	actualMilliseconds := theDuration.Milliseconds()
	if actualMilliseconds != 0 {
		return actualMilliseconds
	}

	if theDuration == 0 {
		return 0
	}

	return 1
}
