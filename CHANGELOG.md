# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-11-24

### Added

- Sync online Omada clients into Pi-hole v6 `dns.hosts` using per-entry add/remove only.
- Ownership by domain suffix; foreign entries are never modified.
- Mass-delete guard, dry-run mode, `--once` (cron) and loop modes, optional DNS restart.
- Multi-arch container image and release binaries.
- Helm chart (CronJob or Deployment), published to `oci://ghcr.io/filipegalo/charts`.

[Unreleased]: https://github.com/filipegalo/omada2pihole/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/filipegalo/omada2pihole/releases/tag/v0.1.0
