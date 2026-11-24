package pihole

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testSID = "SECRET-SID-123"

type piFake struct {
	mu       sync.Mutex
	uris     []string
	logout   int
	putCode  int
	putBody  string
	delCode  int
	hostsErr bool
	badSID   []string
}

func (f *piFake) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uris = append(f.uris, r.Method+" "+r.RequestURI)
	if strings.Contains(r.RequestURI, testSID) {
		f.badSID = append(f.badSID, "sid in URL: "+r.RequestURI)
	}
	if !(r.URL.Path == "/api/auth" && r.Method == http.MethodPost) {
		if r.Header.Get("X-FTL-SID") != testSID {
			f.badSID = append(f.badSID, "missing SID header on "+r.RequestURI)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/api/auth" && r.Method == http.MethodPost:
		_, _ = w.Write([]byte(`{"session":{"valid":true,"totp":false,"sid":"` + testSID + `"}}`))
	case r.URL.Path == "/api/auth" && r.Method == http.MethodDelete:
		f.logout++
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/api/config/dns/hosts" && r.Method == http.MethodGet:
		if f.hostsErr {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"config":{"dns":{"hosts":["10.0.0.1 a.lan"]}}}`))
	case r.Method == http.MethodPut:
		code := f.putCode
		if code == 0 {
			code = 201
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(f.putBody))
	case r.Method == http.MethodDelete:
		code := f.delCode
		if code == 0 {
			code = 204
		}
		w.WriteHeader(code)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newPi(t *testing.T, f *piFake) *Client {
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return New(srv.URL, "pw", srv.Client())
}

func TestPiholePathEscapingAndSIDHeader(t *testing.T) {
	f := &piFake{}
	p := newPi(t, f)
	ctx := context.Background()
	err := p.WithSession(ctx, func() error {
		if _, err := p.Hosts(ctx); err != nil {
			return err
		}
		if err := p.AddHost(ctx, "10.0.0.2 b.lan"); err != nil {
			return err
		}
		return p.DeleteHost(ctx, "10.0.0.1 a.lan")
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(f.uris, "\n")
	if !strings.Contains(joined, "PUT /api/config/dns/hosts/10.0.0.2%20b.lan") {
		t.Errorf("PUT not escaped: %s", joined)
	}
	if !strings.Contains(joined, "DELETE /api/config/dns/hosts/10.0.0.1%20a.lan") {
		t.Errorf("DELETE not escaped: %s", joined)
	}
	if len(f.badSID) > 0 {
		t.Errorf("SID problems: %v", f.badSID)
	}
	if f.logout != 1 {
		t.Errorf("logout calls = %d", f.logout)
	}
}

func TestPiholeAddTolerance(t *testing.T) {
	f := &piFake{putCode: 400, putBody: `{"error":{"key":"bad_request","message":"Item already present","hint":"x"}}`}
	p := newPi(t, f)
	ctx := context.Background()
	_ = p.Login(ctx)
	if err := p.AddHost(ctx, "10.0.0.2 b.lan"); err != nil {
		t.Errorf("already present should be tolerated: %v", err)
	}
	f.putBody = `{"error":{"key":"bad_request","message":"Invalid value"}}`
	if err := p.AddHost(ctx, "bogus"); err == nil || !strings.Contains(err.Error(), "Invalid value") {
		t.Errorf("other 400 must error: %v", err)
	}
	f.putCode, f.putBody = 403, ""
	if err := p.AddHost(ctx, "x"); err == nil || !strings.Contains(err.Error(), "app_sudo") {
		t.Errorf("403 should hint app_sudo: %v", err)
	}
}

func TestPiholeDelete404Tolerated(t *testing.T) {
	f := &piFake{delCode: 404}
	p := newPi(t, f)
	ctx := context.Background()
	_ = p.Login(ctx)
	if err := p.DeleteHost(ctx, "10.0.0.1 a.lan"); err != nil {
		t.Errorf("404 should be tolerated: %v", err)
	}
	f.delCode = 500
	if err := p.DeleteHost(ctx, "10.0.0.1 a.lan"); err == nil {
		t.Error("500 must error")
	}
}

func TestPiholeLogoutOnFailure(t *testing.T) {
	f := &piFake{hostsErr: true}
	p := newPi(t, f)
	ctx := context.Background()
	err := p.WithSession(ctx, func() error {
		_, err := p.Hosts(ctx)
		return err
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if f.logout != 1 {
		t.Errorf("logout calls = %d, want 1", f.logout)
	}
	f.logout = 0
	_ = p.WithSession(ctx, func() error { return errors.New("boom") })
	if f.logout != 1 {
		t.Errorf("logout calls = %d, want 1", f.logout)
	}
}

func TestPiholeLogoutAfterCancelledContext(t *testing.T) {
	f := &piFake{}
	p := newPi(t, f)
	ctx, cancel := context.WithCancel(context.Background())
	_ = p.WithSession(ctx, func() error { cancel(); return ctx.Err() })
	if f.logout != 1 {
		t.Errorf("logout calls = %d, want 1", f.logout)
	}
}

func TestPiholeLoginErrors(t *testing.T) {
	for code, want := range map[int]string{401: "bad password", 429: "rate limited"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		p := New(srv.URL, "supersecret", srv.Client())
		err := p.Login(context.Background())
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "supersecret") {
			t.Errorf("code %d: %v", code, err)
		}
	}
}

func TestPiholePasswordlessLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-FTL-SID") != "" {
			t.Error("unexpected SID")
		}
		_, _ = w.Write([]byte(`{"session":{"valid":true,"totp":false,"sid":null}}`))
	}))
	defer srv.Close()
	p := New(srv.URL, "", srv.Client())
	if err := p.Login(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := p.Logout(); err != nil {
		t.Fatal(err)
	}
}
