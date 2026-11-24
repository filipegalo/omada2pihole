# Contributing

Bug reports, docs fixes and small, focused pull requests are welcome. For larger changes,
please open an issue first.

## Development

Requirements: Go (version in [`go.mod`](go.mod)); Docker is optional.

```sh
make lint    # gofmt check + go vet
make test    # unit tests with the race detector
make build   # static binary in ./bin
make docker  # container image
```

The project uses the Go standard library only. Please don't add third-party dependencies
without discussing it in an issue first.

## Layout

| Path | Purpose |
|---|---|
| `cmd/omada2pihole` | entry point: flags, signals, run loop |
| `internal/config` | environment variable parsing and validation |
| `internal/omada` | Omada OpenAPI client |
| `internal/pihole` | Pi-hole v6 API client |
| `internal/records` | pure logic: naming, ownership, diff, safety guard |
| `internal/syncer` | one sync pass wiring the pieces together |
| `internal/httpx` | shared HTTP client (TLS options, no redirects) |
| `docs/` | user documentation |
| `charts/omada2pihole` | Helm chart; its version is set from the git tag on release |

## Pull requests

- Add tests for behaviour changes. Any change to ownership or deletion logic in
  `internal/records` must include a test proving foreign entries are left untouched.
- Never log or return secrets (client secret, access token, password, session id).
- Add a line to `CHANGELOG.md` under *Unreleased*.
