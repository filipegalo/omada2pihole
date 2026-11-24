package syncer

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/filipegalo/omada2pihole/internal/omada"
	"github.com/filipegalo/omada2pihole/internal/pihole"
)

const omadaClients = `{"errorCode":0,"result":{"totalRows":2,"data":[
 {"name":"Living Room TV","mac":"AA-BB-CC-00-00-01","ip":"10.0.0.10"},
 {"name":"AA-BB-CC-00-00-02","hostName":"laptop","mac":"AA-BB-CC-00-00-02","ip":"10.0.0.11"}]}}`

func fakeOmada(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/authorize/token":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"accessToken":"tok"}}`)
		case "/openapi/v1/oid/sites":
			_, _ = io.WriteString(w, `{"errorCode":0,"result":{"totalRows":1,"data":[{"siteId":"s1","name":"Default"}]}}`)
		case "/openapi/v2/oid/sites/s1/clients":
			_, _ = io.WriteString(w, omadaClients)
		default:
			t.Errorf("unexpected omada path %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakePihole keeps dns.hosts in memory and records every write.
type fakePihole struct {
	mu     sync.Mutex
	hosts  []string
	writes []string
	logout int
}

func (f *fakePihole) handler(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	const prefix = "/api/config/dns/hosts/"
	switch {
	case r.URL.Path == "/api/auth" && r.Method == http.MethodPost:
		_, _ = io.WriteString(w, `{"session":{"valid":true,"sid":"sid"}}`)
	case r.URL.Path == "/api/auth" && r.Method == http.MethodDelete:
		f.logout++
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/api/config/dns/hosts":
		var b strings.Builder
		for i, h := range f.hosts {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(`"` + h + `"`)
		}
		_, _ = io.WriteString(w, `{"config":{"dns":{"hosts":[`+b.String()+`]}}}`)
	case strings.HasPrefix(r.URL.Path, prefix):
		entry, _ := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), prefix))
		f.writes = append(f.writes, r.Method+" "+entry)
		if r.Method == http.MethodPut {
			f.hosts = append(f.hosts, entry)
			w.WriteHeader(http.StatusCreated)
		} else {
			f.hosts = slices.DeleteFunc(f.hosts, func(h string) bool { return h == entry })
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newSyncer(t *testing.T, f *fakePihole, opts Options) *Syncer {
	om := fakeOmada(t)
	ph := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(ph.Close)
	opts.Site, opts.Suffix = "Default", "lan"
	return &Syncer{
		Omada:  omada.New(om.URL, "oid", "cid", "secret", om.Client()),
		Pihole: pihole.New(ph.URL, "pw", ph.Client()),
		Opts:   opts,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestRunOnceEndToEnd(t *testing.T) {
	foreign := []string{
		"10.0.0.1 router.lan router", // alias: foreign
		"10.0.0.2 nas.home",          // other suffix: foreign
		"10.0.0.3 a.b.lan",           // multi-label: foreign
	}
	f := &fakePihole{hosts: append(slices.Clone(foreign), "10.0.0.99 gone.lan", "10.0.0.10 living-room-tv.lan")}
	s := newSyncer(t, f, Options{})

	res, err := s.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"DELETE 10.0.0.99 gone.lan", "PUT 10.0.0.11 laptop.lan"}
	if !slices.Equal(f.writes, want) {
		t.Fatalf("writes=%v want %v", f.writes, want)
	}
	for _, e := range foreign {
		if !slices.Contains(f.hosts, e) {
			t.Errorf("foreign entry %q was removed", e)
		}
	}
	if res != (Result{Added: 1, Removed: 1, Unchanged: 1}) || f.logout != 1 {
		t.Fatalf("res=%+v logout=%d", res, f.logout)
	}

	// Second pass is a no-op.
	f.writes = nil
	res, err = s.RunOnce(context.Background())
	if err != nil || len(f.writes) != 0 || res.Unchanged != 2 {
		t.Fatalf("second pass: res=%+v writes=%v err=%v", res, f.writes, err)
	}
}

func TestRunOnceDryRunWritesNothing(t *testing.T) {
	f := &fakePihole{hosts: []string{"10.0.0.99 gone.lan"}}
	res, err := newSyncer(t, f, Options{DryRun: true}).RunOnce(context.Background())
	if err != nil || len(f.writes) != 0 || res.Added != 2 || res.Removed != 1 {
		t.Fatalf("res=%+v writes=%v err=%v", res, f.writes, err)
	}
}
