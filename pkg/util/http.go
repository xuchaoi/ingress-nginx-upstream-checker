package util

import (
	"crypto/tls"
	"io"
	"net/http"
	"time"
)

func InsecureHttpsGet(url string) (*http.Response, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{
		Transport: tr,
		Timeout: 5 * time.Second,
	}
	res, err := client.Get(url)
	return res, err
}

func HttpGet(url string) (*http.Response, error) {
	client := http.Client{
		Timeout: 1 * time.Second,
	}
	res, err := client.Get(url)
	return res, err
}

func HttpPost(url string, body io.Reader) (*http.Response, error)  {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	res, err := client.Post(url, "application/json", body)
	return res, err
}
