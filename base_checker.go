package main

import (
	"io"
	"strings"
	"sync"
	"time"
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
	Name     string
	Interval time.Duration
	Team     string

	updateDurationMutex sync.Mutex
	parentServer        *Server
}

func (p *BaseUserChecker) GetName() string { return p.Name }

func (p *BaseUserChecker) Enabled() bool { return p.Interval > 0 }

func (p *BaseUserChecker) DoCheck() error {
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
