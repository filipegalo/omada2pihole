// Package syncer runs one Omada → Pi-hole synchronisation pass.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/filipegalo/omada2pihole/internal/omada"
	"github.com/filipegalo/omada2pihole/internal/pihole"
	"github.com/filipegalo/omada2pihole/internal/records"
)

// Options controls a pass.
type Options struct {
	Site            string // Omada site name
	Suffix          string // ownership domain suffix
	DryRun          bool
	RestartDNS      bool // POST /api/action/restartdns after a pass with changes
	AllowMassDelete bool
}

// Result summarises one pass.
type Result struct {
	Added, Removed, Unchanged, Skipped, Failed int
}

// Syncer holds the clients used by every pass.
type Syncer struct {
	Omada  *omada.Client
	Pihole *pihole.Client
	Opts   Options
	Log    *slog.Logger
}

// RunOnce performs one pass and logs a single summary line.
func (s *Syncer) RunOnce(ctx context.Context) (res Result, err error) {
	start := time.Now()
	defer func() {
		attrs := []any{"added", res.Added, "removed", res.Removed, "unchanged", res.Unchanged,
			"skipped", res.Skipped, "failed", res.Failed, "dry_run", s.Opts.DryRun,
			"duration", time.Since(start).Round(time.Millisecond)}
		if err != nil {
			s.Log.Error("sync failed", append(attrs, "err", err)...)
			return
		}
		s.Log.Info("sync complete", attrs...)
	}()

	// Any Omada error aborts the pass before Pi-hole is touched.
	devices, err := s.Omada.FetchClients(ctx, s.Opts.Site)
	if err != nil {
		return res, err
	}
	in := make([]records.Device, len(devices))
	for i, d := range devices {
		in[i] = records.Device(d)
	}
	desired, err := records.Build(in, s.Opts.Suffix)
	if err != nil {
		return res, err
	}

	err = s.Pihole.WithSession(ctx, func() error {
		current, err := s.Pihole.Hosts(ctx)
		if err != nil {
			return err
		}
		plan, err := records.Diff(desired, current, s.Opts.Suffix, s.Opts.AllowMassDelete)
		if err != nil {
			return err
		}
		res.Unchanged, res.Skipped = len(plan.Unchanged), len(plan.Skipped)
		for _, e := range plan.Skipped {
			s.Log.Info("skipped: conflicts with foreign entry", "record", e)
		}
		apply := func(op, entry string, fn func(context.Context, string) error, counter *int) {
			s.Log.Info("plan", "op", op, "record", entry, "dry_run", s.Opts.DryRun)
			if s.Opts.DryRun || ctx.Err() != nil {
				return
			}
			if err := fn(ctx, entry); err != nil {
				res.Failed++
				s.Log.Error("operation failed", "op", op, "record", entry, "err", err)
				return
			}
			*counter++
		}
		// Removes first, so an IP that moves to a new name never has two PTRs.
		for _, e := range plan.ToRemove {
			apply("remove", e, s.Pihole.DeleteHost, &res.Removed)
		}
		for _, e := range plan.ToAdd {
			apply("add", e, s.Pihole.AddHost, &res.Added)
		}
		if s.Opts.DryRun {
			res.Added, res.Removed = len(plan.ToAdd), len(plan.ToRemove)
			return nil
		}
		if s.Opts.RestartDNS && res.Added+res.Removed > 0 {
			return s.Pihole.RestartDNS(ctx)
		}
		return nil
	})
	if err == nil && res.Failed > 0 {
		err = fmt.Errorf("%d operation(s) failed", res.Failed)
	}
	if ctx.Err() != nil { // cancelled mid-pass: remaining operations were not attempted
		err = errors.Join(err, ctx.Err())
	}
	return res, err
}
