package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

type UrlChecker struct {
	Host     string
	Name     string
	Path     string
	Interval time.Duration

	duration time.Duration
	client   http.Client
}

func NewUrlChecker(host, name, path string, interval time.Duration) *UrlChecker {
	return &UrlChecker{
		Host:     host,
		Name:     name,
		Path:     path,
		Interval: interval,
	}
}

func (p *UrlChecker) Enabled() bool { return p.Interval > 0 && p.Path != "" }

func (p *UrlChecker) GetName() string { return p.Name }

func (p *UrlChecker) Report(w io.Writer) {
	if p.Enabled() {
		_, _ = io.WriteString(w, fmt.Sprintf("# TYPE %s gauge\n", p.Name))
		_, _ = io.WriteString(w, fmt.Sprintf("%s{host=\"%s\"} %d\n", p.Name, p.Host, p.duration.Milliseconds()))
	}
}

func (p *UrlChecker) Start() {
	if !p.Enabled() {
		return
	}

	p.client.Timeout = p.Interval

	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		start := time.Now()
		content, err := p.checkContent()
		if err != nil || len(content) == 0 {
			log.Printf("[%s] %s: %s fail: %v", p.Host, p.Name, p.duration.Round(time.Millisecond), err)
			p.duration = p.Interval
			continue
		}

		p.duration = time.Since(start)
		size := humanize.Bytes(uint64(len(content)))
		log.Printf("[%s] %s: %s (%s)", p.Host, p.Name, p.duration.Round(time.Millisecond), size)

		<-ticker.C
	}
}

func (p *UrlChecker) checkContent() ([]byte, error) {
	req, err := http.NewRequest("GET", ForceScheme(p.Host)+p.Path, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request fail")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "client do fail")
	}
	defer resp.Body.Close()

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body fail")
	}

	if resp.StatusCode != 200 {
		return nil, errors.Wrapf(err, "status code: %d %s", resp.StatusCode, string(respData))
	}

	return respData, nil
}
