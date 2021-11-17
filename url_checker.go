package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

type UrlChecker struct {
	BaseUserChecker

	Path string

	duration time.Duration
	client   http.Client

	updateDurationMutex sync.Mutex
}

func NewUrlChecker(name string, path string, interval time.Duration, parentServer *Server) *UrlChecker {
	return &UrlChecker{
		BaseUserChecker: BaseUserChecker{
			Name:         name,
			Interval:     interval,
			parentServer: parentServer,
		},

		Path: path,

		client: http.Client{
			Timeout: time.Second * 10,
		},
	}
}

func (p *UrlChecker) Enabled() bool { return p.Interval > 0 && p.Path != "" }

func (p *UrlChecker) GetInveral() time.Duration {
	return p.Interval
}

func (p *UrlChecker) GetName() string { return p.Name }

func (p *UrlChecker) Report(w io.Writer) {
	if p.Enabled() {
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		_, _ = io.WriteString(w, fmt.Sprintf("# TYPE %s gauge\n", p.Name))
		_, _ = io.WriteString(w, fmt.Sprintf("%s{host=\"%s\"} %d\n", p.Name, p.parentServer.Host, roundMilliseconds(p.duration)))
	}
}

func (p *UrlChecker) DoCheck() error {
	if !p.Enabled() {
		return nil
	}

	var currentDuration time.Duration = 0
	defer func() {
		p.updateDurationMutex.Lock()
		defer p.updateDurationMutex.Unlock()

		p.duration = currentDuration
	}()

	start := time.Now()
	content, err := p.checkContent()
	if err != nil {
		log.Printf("[%s] %s: %dms fail: %v", p.parentServer.Host, p.Name, roundMilliseconds(currentDuration), err)
		currentDuration = 0
		return err
	} else if len(content) == 0 {
		log.Printf("[%s] %s: %dms empty content", p.parentServer.Host, p.Name, roundMilliseconds(currentDuration))
		currentDuration = 0
		return err
	}

	currentDuration = time.Since(start)
	size := humanize.Bytes(uint64(len(content)))
	log.Printf("[%s] %s: %dms (%s)", p.parentServer.Host, p.Name, roundMilliseconds(currentDuration), size)

	return nil
}

func (p *UrlChecker) checkContent() ([]byte, error) {
	req, err := http.NewRequest("GET", ForceScheme(p.parentServer.Host)+p.Path, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request fail")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "client do fail")
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body fail")
	}

	if resp.StatusCode != 200 {
		return nil, errors.Wrapf(err, "status code: %d %s", resp.StatusCode, string(respData))
	}

	return respData, nil
}
