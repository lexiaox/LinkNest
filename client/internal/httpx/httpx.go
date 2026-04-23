package httpx

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// LinkNest clients usually connect to a user-controlled server directly.
			// Bypassing ambient proxy settings avoids SOCKS/HTTP proxy mismatches that surface as EOF.
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     true,
		},
	}
}

func Do(client *http.Client, attempts int, build func() (*http.Request, error)) (*http.Response, error) {
	if client == nil {
		client = NewClient(20 * time.Second)
	}
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := build()
		if err != nil {
			return nil, err
		}

		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		if attempt == attempts-1 || !IsRetryable(err) {
			break
		}
	}

	return nil, lastErr
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return IsRetryable(urlErr.Err)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	return false
}
