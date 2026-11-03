package utils

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

func IsURLEncoded(u string) bool {
	unescaped, err := url.QueryUnescape(u)
	if err != nil {
		return false
	}
	return url.QueryEscape(unescaped) == u
}

func createOptimizedClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        runtime.NumCPU() * 80,
		MaxIdleConnsPerHost: runtime.NumCPU() * 5,
		IdleConnTimeout:     90 * time.Second,

		DisableKeepAlives: false,
		ForceAttemptHTTP2: true,

		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

var httpClient *http.Client = createOptimizedClient()

func GetHTTPClient() *http.Client {
	return httpClient
}
