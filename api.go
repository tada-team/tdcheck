package tdcheck

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

type TdClient struct {
	Host     string
	Insecure bool
	Timeout  time.Duration
}

func (c TdClient) Ping() error {
	resp := new(struct {
		Ok     bool   `json:"ok"`
		Result string `json:"result"`
	})
	_, err := c.doGet("/api/v4/ping", resp)
	return err
}

func (c TdClient) httpClient() *http.Client {
	return &http.Client{
		Timeout: c.Timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func (c TdClient) url(path string) string {
	protocol := "http"
	if !c.Insecure {
		protocol += "s"
	}
	return protocol + "://" + c.Host + path
}

func (c TdClient) doGet(path string, v interface{}) ([]byte, error) {
	client := c.httpClient()

	req, err := http.NewRequest("GET", c.url(path), nil)
	if err != nil {
		return []byte{}, errors.Wrap(err, "new request fail")
	}

	resp, err := client.Do(req)
	if err != nil {
		return []byte{}, errors.Wrap(err, "client do fail")
	}
	defer resp.Body.Close()

	respData, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return respData, errors.Wrap(err, "read body fail")
	}

	if resp.StatusCode != 200 {
		return respData, errors.Wrapf(err, "status code: %d %s", resp.StatusCode, string(respData))
	}

	if err := json.Unmarshal(respData, &v); err != nil {
		return respData, errors.Wrapf(err, "unmarshal fail on: %s", string(respData))
	}

	return respData, nil
}
