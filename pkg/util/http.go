package util

import (
	"crypto/tls"
	"io"
	"net/http"
)

func InsecureHttpsGet(url string) (*http.Response, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{Transport: tr}
	res, err := client.Get(url)
	return res, err
}

func HttpGet(url string) (*http.Response, error) {
	client := http.Client{}
	res, err := client.Get(url)
	return res, err
}

func HttpPost(url string, body io.Reader) (*http.Response, error)  {
	client := http.Client{}
	res, err := client.Post(url, "application/json", body)
	return res, err
}
