package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectNotFollowed(t *testing.T) {
	var leaked bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	hc, err := New(false, "")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/auth", nil)
	req.Header.Set("X-FTL-SID", "secret")
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if leaked || resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("redirect followed: leaked=%v status=%d", leaked, resp.StatusCode)
	}
}

func TestNewRejectsBadCAFile(t *testing.T) {
	if _, err := New(false, "/nonexistent/ca.pem"); err == nil {
		t.Fatal("expected error")
	}
}
