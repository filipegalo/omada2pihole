# omada2pihole

[![CI](https://github.com/filipegalo/omada2pihole/actions/workflows/ci.yml/badge.svg)](https://github.com/filipegalo/omada2pihole/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/filipegalo/omada2pihole)](https://github.com/filipegalo/omada2pihole/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Show device names instead of bare IPs in Pi-hole.

Pi-hole's *Top Clients* and query log name clients through reverse DNS (PTR) lookups. If your
network runs on a TP-Link Omada controller, the names already exist there (aliases you set,
DHCP host names), but the Omada gateway does not answer PTR queries. **omada2pihole**
reads the clients from the Omada OpenAPI and publishes them as Pi-hole
[local DNS records](https://docs.pi-hole.net/), so `10.0.0.11` shows up as `laptop.lan`.

## Features

- Covers every online client, including dynamic DHCP clients, not only reservations.
- Uses the user-edited Omada alias first, then the device host name, then the MAC.
- **Leaves other records alone:** it only adds or removes single entries it owns and never
  replaces Pi-hole's list, so entries made by hand or by tools such as external-dns are kept.
- Safe by default: dry-run mode, a mass-delete guard, no writes when nothing changed.
- Least privilege: a read-only Omada Open API client and a Pi-hole app password.
- Single static binary, no dependencies; small non-root multi-arch image.
- Runs as a loop or as a single pass for cron; Helm chart for Kubernetes.

## Requirements

- Omada software controller with the Open API (written for v6).
- Pi-hole **v6** (REST API).

## Quick start

1. Create an Omada Open API client and a Pi-hole app password — see [docs/setup.md](docs/setup.md).
2. Copy [`examples/omada2pihole.env.example`](examples/omada2pihole.env.example) to
   `omada2pihole.env` and fill it in.
3. Dry run:

   ```sh
   docker run --rm --env-file ./omada2pihole.env -e DRY_RUN=true \
     ghcr.io/filipegalo/omada2pihole:latest --once
   ```

4. Run it for real, every 5 minutes:

   ```sh
   docker run -d --name omada2pihole --restart unless-stopped \
     --env-file ./omada2pihole.env \
     ghcr.io/filipegalo/omada2pihole:latest --interval 5m
   ```

> [!WARNING]
> The tool owns every entry of the form `"<ip> <name>.<DOMAIN_SUFFIX>"` (default suffix `lan`)
> and removes it when Omada no longer reports the client. If you keep hand-made records ending
> in `.lan`, set a dedicated suffix such as `DOMAIN_SUFFIX=omada.lan`.
> See [Ownership](docs/how-it-works.md#ownership).

New names reach DNS immediately. Pi-hole shows them right away for clients it has not seen yet,
and within an hour for clients it already knows — see
[When does Pi-hole show the new name?](docs/how-it-works.md#when-does-pi-hole-show-the-new-name)

## Documentation

| Document | Contents |
|---|---|
| [Setup](docs/setup.md) | Creating the Omada Open API client and the Pi-hole app password |
| [Configuration](docs/configuration.md) | Environment variables, flags, exit codes |
| [How it works](docs/how-it-works.md) | Sync pass, naming rules, ownership, safety guards, name refresh |
| [Deployment](docs/deployment.md) | Docker, Compose, cron, Helm chart, building from source |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please report security issues privately as described in
[SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
