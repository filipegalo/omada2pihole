// Package omada is a minimal client for the TP-Link Omada OpenAPI
// (client-credentials mode).
package omada

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/filipegalo/omada2pihole/internal/httpx"
)

const (
	omadaPageSize = 1000
	omadaMaxPages = 100 // hard stop against a server that ignores "page"
)

// Device is one network client as reported by the controller.
type Device struct {
	Name     string `json:"name"`     // user alias; defaults to the MAC
	HostName string `json:"hostName"` // name reported by the device
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
}

// Client talks to the Omada OpenAPI. The client secret and access token are
// never logged or included in errors.
type Client struct {
	base         string
	omadaID      string
	clientID     string
	clientSecret string
	http         *http.Client
	token        string
}

// New returns a client for the controller at baseURL (scheme://host[:port]).
func New(baseURL, omadaID, clientID, clientSecret string, hc *http.Client) *Client {
	return &Client{base: baseURL, omadaID: omadaID, clientID: clientID, clientSecret: clientSecret, http: hc}
}

type omadaEnvelope struct {
	ErrorCode int             `json:"errorCode"`
	Msg       string          `json:"msg"`
	Result    json.RawMessage `json:"result"`
}

// do performs a request and decodes the envelope. Bodies are never echoed.
func (o *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, o.base+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if o.token != "" {
		req.Header.Set("Authorization", "AccessToken="+o.token)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return fmt.Errorf("omada request %s: %w", httpx.PathOnly(path), httpx.StripURL(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("omada response read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("omada %s: HTTP %d", httpx.PathOnly(path), resp.StatusCode)
	}
	var env omadaEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("omada %s: invalid JSON response", httpx.PathOnly(path))
	}
	if env.ErrorCode != 0 {
		return fmt.Errorf("omada %s: errorCode=%d msg=%q", httpx.PathOnly(path), env.ErrorCode, env.Msg)
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("omada %s: unexpected result shape", httpx.PathOnly(path))
		}
	}
	return nil
}

// Authenticate obtains an access token via the client credentials grant.
func (o *Client) Authenticate(ctx context.Context) error {
	var res struct {
		AccessToken string `json:"accessToken"`
	}
	body := map[string]string{"omadacId": o.omadaID, "client_id": o.clientID, "client_secret": o.clientSecret}
	if err := o.do(ctx, http.MethodPost, "/openapi/authorize/token?grant_type=client_credentials", body, &res); err != nil {
		return fmt.Errorf("omada authentication failed: %w", err)
	}
	if res.AccessToken == "" {
		return errors.New("omada authentication failed: no access token in response")
	}
	o.token = res.AccessToken
	return nil
}

type omadaPage[T any] struct {
	TotalRows int `json:"totalRows"`
	Data      []T `json:"data"`
}

// paginate collects all pages using fetch(page).
func paginate[T any](fetch func(page int) (omadaPage[T], error)) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		if page > omadaMaxPages {
			return nil, fmt.Errorf("omada pagination exceeded %d pages", omadaMaxPages)
		}
		p, err := fetch(page)
		if err != nil {
			return nil, err
		}
		all = append(all, p.Data...)
		if len(p.Data) == 0 || len(all) >= p.TotalRows {
			return all, nil
		}
	}
}

// SiteID resolves a site name to its id (exactly one match required).
func (o *Client) SiteID(ctx context.Context, name string) (string, error) {
	type site struct {
		SiteID string `json:"siteId"`
		Name   string `json:"name"`
	}
	sites, err := paginate(func(page int) (omadaPage[site], error) {
		var p omadaPage[site]
		path := fmt.Sprintf("/openapi/v1/%s/sites?page=%d&pageSize=%d", url.PathEscape(o.omadaID), page, omadaPageSize)
		return p, o.do(ctx, http.MethodGet, path, nil, &p)
	})
	if err != nil {
		return "", err
	}
	var ids []string
	for _, s := range sites {
		if s.Name == name {
			ids = append(ids, s.SiteID)
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", fmt.Errorf("omada site %q not found", name)
	default:
		return "", fmt.Errorf("omada site name %q is ambiguous (%d matches)", name, len(ids))
	}
}

// OnlineClients returns all online clients of the site.
func (o *Client) OnlineClients(ctx context.Context, siteID string) ([]Device, error) {
	return paginate(func(page int) (omadaPage[Device], error) {
		var p omadaPage[Device]
		path := fmt.Sprintf("/openapi/v2/%s/sites/%s/clients", url.PathEscape(o.omadaID), url.PathEscape(siteID))
		body := map[string]int{"page": page, "pageSize": omadaPageSize, "scope": 1}
		return p, o.do(ctx, http.MethodPost, path, body, &p)
	})
}

// FetchClients authenticates, resolves siteName and returns its online clients.
func (o *Client) FetchClients(ctx context.Context, siteName string) ([]Device, error) {
	if err := o.Authenticate(ctx); err != nil {
		return nil, err
	}
	id, err := o.SiteID(ctx, siteName)
	if err != nil {
		return nil, err
	}
	return o.OnlineClients(ctx, id)
}
