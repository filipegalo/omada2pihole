// Command omada2pihole keeps Pi-hole local DNS records in sync with the
// clients known to a TP-Link Omada controller.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/filipegalo/omada2pihole/internal/config"
	"github.com/filipegalo/omada2pihole/internal/httpx"
	"github.com/filipegalo/omada2pihole/internal/omada"
	"github.com/filipegalo/omada2pihole/internal/pihole"
	"github.com/filipegalo/omada2pihole/internal/syncer"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const (
	exitOK     = 0
	exitFailed = 1
	exitConfig = 2
)

func main() { os.Exit(run(os.Args[1:], os.Getenv)) }

func run(args []string, getenv func(string) string) int {
	fs := flag.NewFlagSet("omada2pihole", flag.ContinueOnError)
	once := fs.Bool("once", false, "run a single pass and exit (for cron)")
	interval := fs.Duration("interval", 0, "sync interval (overrides SYNC_INTERVAL)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	if *showVersion {
		fmt.Println(version)
		return exitOK
	}

	cfg, err := config.Load(getenv, *interval, *once)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error:\n%v\n", err)
		return exitConfig
	}
	omadaHTTP, err := httpx.New(cfg.OmadaSkipVerify, cfg.OmadaCAFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: omada TLS: %v\n", err)
		return exitConfig
	}
	piholeHTTP, err := httpx.New(cfg.PiholeSkipVerify, cfg.PiholeCAFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: pihole TLS: %v\n", err)
		return exitConfig
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	logger.Info("starting", "version", version, "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	s := &syncer.Syncer{
		Omada:  omada.New(cfg.OmadaURL, cfg.OmadaID, cfg.OmadaClientID, cfg.OmadaClientSecret, omadaHTTP),
		Pihole: pihole.New(cfg.PiholeURL, cfg.PiholePassword, piholeHTTP),
		Opts: syncer.Options{
			Site:            cfg.OmadaSite,
			Suffix:          cfg.DomainSuffix,
			DryRun:          cfg.DryRun,
			RestartDNS:      cfg.PiholeRestart,
			AllowMassDelete: cfg.AllowMassDelete,
		},
		Log: logger,
	}

	if *once {
		if _, err := s.RunOnce(ctx); err != nil {
			return exitFailed
		}
		return exitOK
	}
	for {
		_, _ = s.RunOnce(ctx) // failures are logged; the loop keeps going
		select {
		case <-ctx.Done():
			return exitOK
		case <-time.After(cfg.Interval):
		}
	}
}
