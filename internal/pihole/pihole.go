// Package pihole is a minimal client for the Pi-hole v6 REST API, limited to
// sessions and single-entry edits of dns.hosts.
package pihole

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filipegalo/omada2pihole/internal/httpx"
)

const logoutTimeout = 10 * time.Second

// Client talks to the Pi-hole v6 API. The SID is sent only in the
// X-FTL-SID header and never appears in URLs, logs or errors.
type Client struct {
	base     string
	password string
	http     *http.Client
	sid      string
}

// New returns a client for the Pi-hole at baseURL (scheme://host[:port]).
// An empty password means the Pi-hole has no password set.
func New(baseURL, password string, hc *http.Client) *Client {
	return &Client{base: baseURL, password: password, http: hc}
}

// apiError is a non-2xx response with Pi-hole's error message (if any).
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("pihole: HTTP %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("pihole: HTTP %d", e.Status)
}

// call performs a request. A non-nil out receives the decoded JSON body.
// Non-2xx responses return *apiError.
func (p *Client) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.sid != "" {
		req.Header.Set("X-FTL-SID", p.sid)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("pihole %s %s: %w", method, httpx.PathOnly(path), httpx.StripURL(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		return &apiError{Status: resp.StatusCode, Message: e.Error.Message}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("pihole %s %s: invalid JSON response", method, httpx.PathOnly(path))
		}
	}
	return nil
}

func statusOf(err error) int {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status
	}
	return 0
}

// Login authenticates. With an empty password and a null SID, no session is used.
func (p *Client) Login(ctx context.Context) error {
	var res struct {
		Session struct {
			Valid   bool    `json:"valid"`
			TOTP    bool    `json:"totp"`
			SID     *string `json:"sid"`
			Message string  `json:"message"`
		} `json:"session"`
	}
	err := p.call(ctx, http.MethodPost, "/api/auth", map[string]string{"password": p.password}, &res)
	switch statusOf(err) {
	case 0:
	case http.StatusUnauthorized:
		if res.Session.TOTP {
			return errors.New("pihole login failed: 2FA is enabled; use an app password")
		}
		return errors.New("pihole login failed: bad password (HTTP 401)")
	case http.StatusTooManyRequests:
		return errors.New("pihole login failed: rate limited or no free API seats (HTTP 429); check webserver.api.max_sessions")
	}
	if err != nil {
		return fmt.Errorf("pihole login failed: %w", err)
	}
	if res.Session.SID != nil && *res.Session.SID != "" {
		if !res.Session.Valid {
			if res.Session.TOTP {
				return errors.New("pihole login failed: 2FA is enabled; use an app password")
			}
			return errors.New("pihole login failed: session not valid")
		}
		p.sid = *res.Session.SID
		return nil
	}
	if res.Session.TOTP {
		return errors.New("pihole login failed: 2FA is enabled; use an app password")
	}
	if !res.Session.Valid {
		return errors.New("pihole login failed: no session returned (bad password?)")
	}
	return nil // passwordless Pi-hole
}

// Logout ends the session; 404 is ignored. Uses its own short context.
func (p *Client) Logout() error {
	if p.sid == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), logoutTimeout)
	defer cancel()
	err := p.call(ctx, http.MethodDelete, "/api/auth", nil, nil)
	p.sid = ""
	if statusOf(err) == http.StatusNotFound {
		return nil
	}
	return err
}

// WithSession logs in, runs fn, and always logs out after a successful login.
func (p *Client) WithSession(ctx context.Context, fn func() error) (err error) {
	if err := p.Login(ctx); err != nil {
		return err
	}
	defer func() {
		if lerr := p.Logout(); lerr != nil && err == nil {
			err = fmt.Errorf("pihole logout: %w", lerr)
		}
	}()
	return fn()
}

// Hosts returns config.dns.hosts.
func (p *Client) Hosts(ctx context.Context) ([]string, error) {
	var res struct {
		Config struct {
			DNS struct {
				Hosts []string `json:"hosts"`
			} `json:"dns"`
		} `json:"config"`
	}
	if err := p.call(ctx, http.MethodGet, "/api/config/dns/hosts", nil, &res); err != nil {
		return nil, fmt.Errorf("reading dns.hosts: %w", err)
	}
	return res.Config.DNS.Hosts, nil
}

func hostPath(entry string) string {
	return "/api/config/dns/hosts/" + url.PathEscape(entry)
}

const sudoHint = "app password needs webserver.api.app_sudo=true to write config"

// AddHost PUTs one entry; "already present" counts as success.
func (p *Client) AddHost(ctx context.Context, entry string) error {
	err := p.call(ctx, http.MethodPut, hostPath(entry), nil, nil)
	if err == nil {
		return nil
	}
	var ae *apiError
	if errors.As(err, &ae) {
		switch {
		case ae.Status == http.StatusBadRequest && strings.Contains(strings.ToLower(ae.Message), "already present"):
			return nil
		case ae.Status == http.StatusForbidden:
			return fmt.Errorf("add %q: %w (%s)", entry, err, sudoHint)
		}
	}
	return fmt.Errorf("add %q: %w", entry, err)
}

// DeleteHost DELETEs one entry; 404 counts as success.
func (p *Client) DeleteHost(ctx context.Context, entry string) error {
	err := p.call(ctx, http.MethodDelete, hostPath(entry), nil, nil)
	if err == nil || statusOf(err) == http.StatusNotFound {
		return nil
	}
	if statusOf(err) == http.StatusForbidden {
		return fmt.Errorf("remove %q: %w (%s)", entry, err, sudoHint)
	}
	return fmt.Errorf("remove %q: %w", entry, err)
}

// RestartDNS restarts FTL's DNS resolver.
func (p *Client) RestartDNS(ctx context.Context) error {
	err := p.call(ctx, http.MethodPost, "/api/action/restartdns", nil, nil)
	if statusOf(err) == http.StatusForbidden {
		return fmt.Errorf("restartdns: %w (requires webserver.api.allow_destructive=true)", err)
	}
	return err
}
