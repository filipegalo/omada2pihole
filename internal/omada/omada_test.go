package omada

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(url string, hc *http.Client) *Client {
	return New(url, "oid", "cid", "TOPSECRET", hc)
}

func TestOmadaErrorCodeWithHTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errorCode":-44106,"msg":"Invalid client credentials","result":null}`))
	}))
	defer srv.Close()
	o := newTestClient(srv.URL, srv.Client())
	err := o.Authenticate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "-44106") || strings.Contains(err.Error(), "TOPSECRET") {
		t.Fatalf("err=%v", err)
	}
}

func TestOmadaFullFlowAndPagination(t *testing.T) {
	var clientPages []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/openapi/authorize/token":
			if r.URL.Query().Get("grant_type") != "client_credentials" {
				t.Error("grant_type missing")
			}
			var b map[string]string
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["omadacId"] != "oid" || b["client_id"] != "cid" || b["client_secret"] != "TOPSECRET" {
				t.Errorf("bad token body %v", b)
			}
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"accessToken":"tok123"}}`))
			return
		}
		if r.Header.Get("Authorization") != "AccessToken=tok123" {
			t.Errorf("bad auth header on %s: %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/openapi/v1/oid/sites":
			_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":2,"data":[{"siteId":"s0","name":"Other"},{"siteId":"s1","name":"Default"}]}}`))
		case "/openapi/v2/oid/sites/s1/clients":
			var b map[string]int
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b["scope"] != 1 {
				t.Errorf("scope=%d", b["scope"])
			}
			clientPages = append(clientPages, b["page"])
			switch b["page"] {
			case 1:
				_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":3,"data":[{"name":"a","mac":"m1","ip":"10.0.0.1"},{"name":"b","mac":"m2","ip":"10.0.0.2"}]}}`))
			case 2:
				_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":3,"data":[{"hostName":"c","mac":"m3","ip":"10.0.0.3"}]}}`))
			default:
				t.Errorf("unexpected page %d", b["page"])
				_, _ = w.Write([]byte(`{"errorCode":0,"result":{"totalRows":3,"data":[]}}`))
			}
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	o := newTestClient(srv.URL, srv.Client())
	cl, err := o.FetchClients(context.Background(), "Default")
	if err != nil {
		t.Fatal(err)
	}
	if len(cl) != 3 || cl[2].HostName != "c" || fmt.Sprint(clientPages) != "[1 2]" {
		t.Fatalf("clients=%v pages=%v", cl, clientPages)
	}
}

func TestOmadaSiteNotFoundAndAmbiguous(t *testing.T) {
	for body, want := range map[string]string{
		`{"errorCode":0,"result":{"totalRows":1,"data":[{"siteId":"s0","name":"Other"}]}}`:                                  "not found",
		`{"errorCode":0,"result":{"totalRows":2,"data":[{"siteId":"a","name":"Default"},{"siteId":"b","name":"Default"}]}}`: "ambiguous",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		o := newTestClient(srv.URL, srv.Client())
		_, err := o.SiteID(context.Background(), "Default")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q got %v", want, err)
		}
	}
}
