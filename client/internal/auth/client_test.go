package auth

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestLoginRetriesRetryableError(t *testing.T) {
	attempts := 0
	client := &Client{
		BaseURL: "http://example.com",
		HTTP: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				if req.URL.Path != "/api/auth/login" {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				if attempts == 1 {
					return nil, io.EOF
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"token":"token-1","user":{"id":1,"username":"test","email":"1"}}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	result, err := client.Login(LoginInput{Username: "test", Password: "1"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if result.Token != "token-1" {
		t.Fatalf("unexpected token: %s", result.Token)
	}
}

func TestRegisterFallsBackToLoginAfterRetryableError(t *testing.T) {
	attempts := 0
	client := &Client{
		BaseURL: "http://example.com",
		HTTP: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				switch attempts {
				case 1:
					if req.URL.Path != "/api/auth/register" {
						t.Fatalf("unexpected path on register attempt: %s", req.URL.Path)
					}
					return nil, io.EOF
				case 2:
					if req.URL.Path != "/api/auth/login" {
						t.Fatalf("unexpected path on fallback login attempt: %s", req.URL.Path)
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(`{"token":"token-2","user":{"id":2,"username":"test","email":"1"}}`)),
						Header:     make(http.Header),
					}, nil
				default:
					t.Fatalf("unexpected request count: %d", attempts)
					return nil, nil
				}
			}),
		},
	}

	result, err := client.Register(RegisterInput{Username: "test", Email: "1", Password: "1"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if result.Token != "token-2" {
		t.Fatalf("unexpected token: %s", result.Token)
	}
	if result.Notice == "" {
		t.Fatal("expected recovery notice to be populated")
	}
}
