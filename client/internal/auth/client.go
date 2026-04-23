package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"linknest/client/internal/httpx"
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
	Token  string `json:"token"`
	User   User   `json:"user"`
	Notice string `json:"-"`
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
		HTTP:    httpx.NewClient(20 * time.Second),
	}
}

func (c *Client) Register(input RegisterInput) (AuthResult, error) {
	result, err := c.postJSON("/api/auth/register", input, 1)
	if err == nil {
		return result, nil
	}
	if !httpx.IsRetryable(err) {
		return AuthResult{}, err
	}

	loginResult, loginErr := c.Login(LoginInput{
		Username: input.Username,
		Password: input.Password,
	})
	if loginErr == nil {
		loginResult.Notice = "register request was interrupted after reaching the server; the account is already available and this login was recovered automatically"
		return loginResult, nil
	}

	return AuthResult{}, err
}

func (c *Client) Login(input LoginInput) (AuthResult, error) {
	return c.postJSON("/api/auth/login", input, 2)
}

func (c *Client) postJSON(path string, payload interface{}, attempts int) (AuthResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return AuthResult{}, err
	}

	resp, err := httpx.Do(c.HTTP, attempts, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
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
