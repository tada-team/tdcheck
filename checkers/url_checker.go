package checkers

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

func NewUrlChecker(prefix, name, url string, interval time.Duration) *UrlChecker {
	return &UrlChecker{
		host:     prefix,
		name:     name,
		url:      url,
		interval: interval,
	}
}

type UrlChecker struct {
	host     string
	name     string
	url      string
	interval time.Duration
	duration time.Duration
}

func (p *UrlChecker) Enabled() bool { return p.interval > 0 && p.url != "" }

func (p *UrlChecker) Start() {
	if !p.Enabled() {
		return
	}

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		start := time.Now()
		content, err := checkContent(p.url)
		if err != nil || len(content) == 0 {
			log.Printf("[%s] %s: %s fail: %v", p.host, p.name, p.duration.Truncate(time.Millisecond), err)
			p.duration = p.interval
			continue
		}

		p.duration = time.Since(start)
		size := humanize.Bytes(uint64(len(content)))
		log.Printf("[%s] %s: %s (%s)", p.host, p.name, p.duration.Truncate(time.Millisecond), size)

		<-ticker.C
	}
}

func (p *UrlChecker) Report(w http.ResponseWriter) {
	if p.Enabled() {
		_, _ = io.WriteString(w, fmt.Sprintf("# TYPE %s gauge\n", p.name))
		_, _ = io.WriteString(w, fmt.Sprintf("%s{host=\"%s\"} %d\n", p.name, p.host, p.duration.Milliseconds()))
	}
}

func checkContent(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "new request fail")
	}

	resp, err := http.DefaultClient.Do(req)
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
