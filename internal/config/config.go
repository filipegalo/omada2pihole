// Package config loads and validates the environment-variable configuration.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/filipegalo/omada2pihole/internal/records"
)

// MinInterval is the shortest allowed loop interval.
const MinInterval = 30 * time.Second

// Config holds all runtime settings. Secrets are redacted by String/LogValue.
type Config struct {
	OmadaURL          string
	OmadaID           string
	OmadaClientID     string
	OmadaClientSecret string
	OmadaSite         string
	OmadaSkipVerify   bool
	OmadaCAFile       string

	PiholeURL        string
	PiholePassword   string
	PiholeSkipVerify bool
	PiholeCAFile     string

	DomainSuffix    string
	Interval        time.Duration
	DryRun          bool
	PiholeRestart   bool
	AllowMassDelete bool
	LogLevel        slog.Level
}

// Load reads configuration from getenv. intervalOverride (if > 0)
// replaces SYNC_INTERVAL. The interval minimum is only enforced when !once.
func Load(getenv func(string) string, intervalOverride time.Duration, once bool) (*Config, error) {
	var errs []error
	get := func(k, def string) string {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			return v
		}
		return def
	}
	req := func(k string) string {
		v := strings.TrimSpace(getenv(k))
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required", k))
		}
		return v
	}
	boolean := func(k string) bool {
		v := strings.TrimSpace(getenv(k))
		if v == "" {
			return false
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: invalid boolean", k))
		}
		return b
	}
	baseURL := func(k, v string) string {
		if v == "" {
			return v
		}
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s: must be an http(s) URL", k))
			return ""
		}
		// Userinfo would be logged with the URL; a path/query would break API paths.
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			errs = append(errs, fmt.Errorf("%s: must be scheme://host[:port] only (no credentials, path or query)", k))
			return ""
		}
		return strings.TrimRight(v, "/")
	}

	c := &Config{
		OmadaURL:          baseURL("OMADA_URL", req("OMADA_URL")),
		OmadaID:           req("OMADA_ID"),
		OmadaClientID:     req("OMADA_CLIENT_ID"),
		OmadaClientSecret: req("OMADA_CLIENT_SECRET"),
		OmadaSite:         get("OMADA_SITE", "Default"),
		OmadaSkipVerify:   boolean("OMADA_TLS_SKIP_VERIFY"),
		OmadaCAFile:       get("OMADA_CA_FILE", ""),
		PiholeURL:         baseURL("PIHOLE_URL", req("PIHOLE_URL")),
		PiholePassword:    getenv("PIHOLE_PASSWORD"),
		PiholeSkipVerify:  boolean("PIHOLE_TLS_SKIP_VERIFY"),
		PiholeCAFile:      get("PIHOLE_CA_FILE", ""),
		DryRun:            boolean("DRY_RUN"),
		PiholeRestart:     boolean("PIHOLE_RESTART_DNS"),
		AllowMassDelete:   boolean("ALLOW_MASS_DELETE"),
	}
	if c.OmadaSkipVerify && c.OmadaCAFile != "" {
		errs = append(errs, errors.New("OMADA_TLS_SKIP_VERIFY and OMADA_CA_FILE are mutually exclusive"))
	}
	if c.PiholeSkipVerify && c.PiholeCAFile != "" {
		errs = append(errs, errors.New("PIHOLE_TLS_SKIP_VERIFY and PIHOLE_CA_FILE are mutually exclusive"))
	}

	suffix, err := normalizeSuffix(get("DOMAIN_SUFFIX", "lan"))
	if err != nil {
		errs = append(errs, err)
	}
	c.DomainSuffix = suffix

	c.Interval = 5 * time.Minute
	if v := get("SYNC_INTERVAL", ""); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, errors.New("SYNC_INTERVAL: invalid duration"))
		} else {
			c.Interval = d
		}
	}
	if intervalOverride > 0 {
		c.Interval = intervalOverride
	}
	if !once && c.Interval < MinInterval {
		errs = append(errs, fmt.Errorf("sync interval must be at least %s", MinInterval))
	}

	switch strings.ToLower(get("LOG_LEVEL", "info")) {
	case "info":
		c.LogLevel = slog.LevelInfo
	case "debug":
		c.LogLevel = slog.LevelDebug
	default:
		errs = append(errs, errors.New("LOG_LEVEL: must be debug or info"))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

// normalizeSuffix lowercases, trims surrounding dots and validates labels.
func normalizeSuffix(s string) (string, error) {
	s = strings.Trim(strings.ToLower(strings.TrimSpace(s)), ".")
	if s == "" || len(s) > 200 {
		return "", errors.New("DOMAIN_SUFFIX: invalid")
	}
	for _, l := range strings.Split(s, ".") {
		if !records.ValidLabel(l) {
			return "", fmt.Errorf("DOMAIN_SUFFIX: invalid label %q", l)
		}
	}
	return s, nil
}

func redact(s string) string {
	if s == "" {
		return "<empty>"
	}
	return "<redacted>"
}

// String renders the config with secrets redacted.
func (c Config) String() string {
	return fmt.Sprintf("omada_url=%s omada_id=%s omada_client_id=%s omada_client_secret=%s omada_site=%s "+
		"omada_tls_skip_verify=%t omada_ca_file=%q pihole_url=%s pihole_password=%s pihole_tls_skip_verify=%t "+
		"pihole_ca_file=%q domain_suffix=%s interval=%s dry_run=%t pihole_restart_dns=%t allow_mass_delete=%t",
		c.OmadaURL, c.OmadaID, c.OmadaClientID, redact(c.OmadaClientSecret), c.OmadaSite,
		c.OmadaSkipVerify, c.OmadaCAFile, c.PiholeURL, redact(c.PiholePassword), c.PiholeSkipVerify,
		c.PiholeCAFile, c.DomainSuffix, c.Interval, c.DryRun, c.PiholeRestart, c.AllowMassDelete)
}

// LogValue implements slog.LogValuer with secrets redacted.
func (c Config) LogValue() slog.Value { return slog.StringValue(c.String()) }
