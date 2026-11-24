# How it works

## One sync pass

```
Omada OpenAPI                         omada2pihole                        Pi-hole v6 API
─────────────                         ─────────────────                        ──────────────
POST /openapi/authorize/token  ◄────  1. get access token
GET  /openapi/v1/{id}/sites    ◄────  2. resolve site name → site id
POST /openapi/v2/{id}/sites/          3. list online clients (paginated)
     {site}/clients            ◄────
                                      4. build desired records
                                      5. log in ──────────────────────────────►  POST /api/auth
                                      6. read current list ───────────────────►  GET  /api/config/dns/hosts
                                      7. diff (owned entries only)
                                      8. remove stale, then add new ──────────►  DELETE / PUT /api/config/dns/hosts/{entry}
                                      9. log out (always) ────────────────────►  DELETE /api/auth
                                     10. log one summary line
```

Any Omada error aborts the pass before Pi-hole is touched. Pi-hole writes are single-entry
calls only; the whole `dns.hosts` list is never replaced.

A summary line is logged for every pass, successful or not:

```
level=INFO msg="sync complete" added=1 removed=0 unchanged=23 skipped=0 failed=0 dry_run=false duration=184ms
```

## Naming

Each online client with an IPv4 address becomes one record `"<ip> <label>.<suffix>"`.

The label comes from the first usable source:

1. the Omada **alias** (the name you edit in the controller), unless it is just the MAC;
2. the **host name** reported by the device (only the part before the first dot, so
   `laptop.home.lan` → `laptop`);
3. the **MAC** (`aa-bb-cc-dd-ee-ff`);
4. the **IP** (`ip-10-0-0-6`).

Labels are lowercased, characters outside `[a-z0-9-]` become `-`, runs of `-` are collapsed,
leading and trailing `-` are trimmed, and the result is cut to 63 characters.

**Duplicates** are resolved deterministically: when two or more clients end up with the same
label, *all* of them get `-<last IPv4 octet>` appended (`tv-10`, `tv-11`). The result does not
depend on the order the controller returns clients in. In the rare case that still collides,
the MAC is appended.

IPv6 addresses are not synced.

## Ownership

An entry in `dns.hosts` is **owned** by this tool only if it is exactly
`"<ipv4> <label>.<DOMAIN_SUFFIX>"`: canonical IPv4, a single space, one name, exactly one label
in front of the suffix. Everything else is **foreign** and is never modified or removed:

| Entry | Owned with `DOMAIN_SUFFIX=lan`? |
|---|---|
| `10.0.0.5 tv.lan` | yes |
| `10.0.0.5 tv.lan tv` (alias) | no |
| `10.0.0.5 tv.home` (other suffix) | no |
| `10.0.0.5 tv.living.lan` (multi-label) | no |
| `10.0.0.5  tv.lan` (two spaces) | no |
| `fd00::5 tv.lan` (IPv6) | no |

If a foreign entry already uses the IP or the name of a record the tool wants to add, that
record is **skipped** (counted as `skipped`). dnsmasq answers a PTR query with the *first*
entry for an IP, so adding a second one would not change the name Pi-hole shows anyway.

> [!WARNING]
> With the default suffix `lan`, a hand-made entry such as `192.168.1.5 nas.lan` looks exactly
> like an owned entry and is **removed** when Omada does not report that client (for example
> while it is offline). If you keep manual records ending in `.lan`, use a dedicated suffix such
> as `DOMAIN_SUFFIX=omada.lan`.

**Changing the suffix later:** old-suffix entries become foreign, so they are not cleaned up,
and the new records are skipped because their IPs collide with the old ones. Delete the
old-suffix entries in Pi-hole (*Settings → Local DNS Records*) after switching.

## Safety guards

- **Mass-delete guard.** A pass is refused, with nothing written and a non-zero exit, if it
  would remove *all* owned entries, or more than half of them when that is more than 5.
  This protects against a partial client list, for example right after the controller
  restarts. If the removal is intended, run once with `ALLOW_MASS_DELETE=true`.
- **Idempotent.** When nothing changed, no write calls are made.
- **Removes before adds**, so an IP that moves to a new name never briefly has two records.
- **No redirects** are followed, so the password, client secret or session id can never be
  forwarded to another host.
- **Secrets** are redacted from logs; the Pi-hole session id travels only in the `X-FTL-SID`
  header, never in a URL.
- The Pi-hole session is **always logged out** (Pi-hole allows 16 concurrent API sessions by
  default), including after errors and on SIGTERM.

## When does Pi-hole show the new name?

- `dns.hosts` changes are live immediately: Pi-hole rewrites its hosts file and dnsmasq
  re-reads it. No restart is needed for DNS lookups.
- Pi-hole resolves the name of a **new** client (first seen by Pi-hole) within about a second.
- **Existing** clients are re-resolved hourly, at the top of the hour, if they were active in
  the last 2 hours. This is controlled by `resolver.refreshNames` (default `IPV4_ONLY`;
  `UNKNOWN` only refreshes clients without a name; `NONE` never refreshes). A failed lookup
  never clears an existing name.
- Pi-hole has **no API endpoint** to trigger a name refresh. Your options:
  - wait up to an hour;
  - on the Pi-hole host, run `sudo bash -c 'kill -s RTMIN+4 $(pidof pihole-FTL)'` (forces
    re-resolution without a restart, still respects `refreshNames`);
  - restart FTL: `sudo systemctl restart pihole-FTL`, or *Settings → System → Restart DNS
    resolver* in the web interface (v6's `pihole` command has no `restartdns`);
  - set `PIHOLE_RESTART_DNS=true`: after a pass that changed something, the tool calls
    `POST /api/action/restartdns`. This restarts FTL (a short DNS interruption) and requires
    `webserver.api.allow_destructive=true`. Re-resolution after a restart is expected but not
    verified.

## Why not conditional forwarding?

Pi-hole could forward reverse lookups to the router (`dns.revServers`). The Omada gateway,
however, answers PTR queries with an authoritative NXDOMAIN, even for clients with a DHCP
reservation, so this does not work. Publishing the names as Pi-hole local records does.
