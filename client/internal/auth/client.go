package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type RegisterInput struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResult struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) Register(input RegisterInput) (AuthResult, error) {
	return c.postJSON("/api/auth/register", input)
}

func (c *Client) Login(input LoginInput) (AuthResult, error) {
	return c.postJSON("/api/auth/login", input)
}

func (c *Client) postJSON(path string, payload interface{}) (AuthResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return AuthResult{}, err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return AuthResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return AuthResult{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return AuthResult{}, err
	}

	if resp.StatusCode >= 400 {
		var remoteErr errorBody
		if err := json.Unmarshal(body, &remoteErr); err == nil && remoteErr.Error.Message != "" {
			return AuthResult{}, fmt.Errorf("%s: %s", remoteErr.Error.Code, remoteErr.Error.Message)
		}
		return AuthResult{}, fmt.Errorf("request failed: %s", resp.Status)
	}

	var result AuthResult
	if err := json.Unmarshal(body, &result); err != nil {
		return AuthResult{}, err
	}
	return result, nil
}
