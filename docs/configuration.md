# Configuration

All configuration comes from environment variables. A template is in
[`examples/omada2pihole.env.example`](../examples/omada2pihole.env.example).

## Environment variables

### Omada

| Variable | Default | Description |
|---|---|---|
| `OMADA_URL` | *required* | Controller address, e.g. `https://omada.local:8043`. Scheme, host and port only: no credentials, path or query. |
| `OMADA_ID` | *required* | The Omada ID (omadacId) shown on the Open API credential page. |
| `OMADA_CLIENT_ID` | *required* | Open API client ID. |
| `OMADA_CLIENT_SECRET` | *required* | Open API client secret. Never logged. |
| `OMADA_SITE` | `Default` | Site **name**; it is resolved to a site ID. |
| `OMADA_TLS_SKIP_VERIFY` | `false` | Skip TLS certificate verification (self-signed controller). |
| `OMADA_CA_FILE` | *empty* | PEM file with the CA that signed the controller certificate. Cannot be combined with `OMADA_TLS_SKIP_VERIFY`. |

### Pi-hole

| Variable | Default | Description |
|---|---|---|
| `PIHOLE_URL` | *required* | e.g. `https://pi.hole`. Scheme, host and port only. |
| `PIHOLE_PASSWORD` | *empty* | App password or web password. Empty means the Pi-hole has no password. Never logged. |
| `PIHOLE_TLS_SKIP_VERIFY` | `false` | Skip TLS certificate verification. |
| `PIHOLE_CA_FILE` | *empty* | PEM CA file. Cannot be combined with `PIHOLE_TLS_SKIP_VERIFY`. |
| `PIHOLE_RESTART_DNS` | `false` | After a pass that changed something, call `restartdns` so Pi-hole re-resolves client names. Needs `webserver.api.allow_destructive=true`. See [When does Pi-hole show the new name?](how-it-works.md#when-does-pi-hole-show-the-new-name) |

### Behaviour

| Variable | Default | Description |
|---|---|---|
| `DOMAIN_SUFFIX` | `lan` | Suffix of every record and the marker of ownership. Lowercased; surrounding dots removed. Read the [ownership warning](how-it-works.md#ownership). |
| `SYNC_INTERVAL` | `5m` | Go duration between passes in loop mode. Minimum `30s` (not checked with `--once`). |
| `DRY_RUN` | `false` | Log the planned changes without writing anything. |
| `ALLOW_MASS_DELETE` | `false` | Bypass the [mass-delete guard](how-it-works.md#safety-guards). Use it for a single run only. |
| `LOG_LEVEL` | `info` | `info` or `debug`. |

Booleans accept `true`/`false`/`1`/`0` (Go `strconv.ParseBool`); anything else is a
configuration error.

## Flags

| Flag | Description |
|---|---|
| `--once` | Run a single pass and exit (for cron and Kubernetes CronJobs). |
| `--interval <duration>` | Loop interval; overrides `SYNC_INTERVAL`. |
| `--version` | Print the version and exit. |

Without `--once`, the tool runs a pass immediately and then once per interval until it gets
SIGINT or SIGTERM. Failed passes are logged and the loop continues.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Success (or clean shutdown in loop mode). |
| `1` | The pass failed, including partial failures and a refused mass delete. |
| `2` | Configuration error. |
