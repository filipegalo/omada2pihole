// Package httpx builds the HTTP clients used to talk to Omada and Pi-hole and
// provides helpers that keep secrets out of error messages.
package httpx

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Timeout bounds every single request.
const Timeout = 15 * time.Second

// New returns an HTTP client with the given TLS settings. skipVerify and
// caFile are mutually exclusive; that is validated by the config package.
//
// Redirects are never followed: Go would re-send custom headers (such as
// X-FTL-SID) and request bodies (passwords, client secrets) to the redirect
// target. A 3xx response is returned to the caller, which treats it as an error.
func New(skipVerify bool, caFile string) (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if skipVerify {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // explicit opt-in by the operator
	}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("CA file %s contains no valid certificates", caFile)
		}
		tlsCfg.RootCAs = pool
	}
	tr.TLSClientConfig = tlsCfg
	return &http.Client{
		Transport:     tr,
		Timeout:       Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

// PathOnly returns the path of p without its query string.
func PathOnly(p string) string {
	if u, err := url.Parse(p); err == nil {
		return u.Path
	}
	return p
}

// StripURL unwraps *url.Error so the request URL is not repeated in the
// message; callers add the path themselves.
func StripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
