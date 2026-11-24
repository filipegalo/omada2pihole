# Setup

You need two sets of credentials: an Omada Open API client and a Pi-hole password.

## Omada: create an Open API client

The tool uses the Omada OpenAPI in **client mode** (client credentials). Create a dedicated,
least-privilege credential for it.

1. In the controller, open **Global View → Settings → Platform Integration → Open API** and
   choose **Add New Credential**.
2. Set **Mode** to **Client**.
3. **Role**: create a custom role that has only view access to site clients ("Site Clients
   Manager — View Only"); the tool never writes to the controller.
4. **Site Privileges**: only the site you want to sync.
5. Save, then open the credential (**View**) and copy:
   - **Client ID** → `OMADA_CLIENT_ID`
   - **Client Secret** → `OMADA_CLIENT_SECRET` (it is shown only once)
   - **Omada ID** → `OMADA_ID`
   - **Interface Access Address** → `OMADA_URL`

Notes:

- Use the Omada ID from the credential page. The value from `/api/info` does not work with the
  Open API on some controller versions.
- Changing the credential's role or site privileges invalidates its tokens; the tool fetches a
  new token every pass, so it recovers on its own.
- Open API availability and menu names vary between controller versions; this was written for
  the v6 software controller.

## Pi-hole: password

The tool needs a password that can **change configuration**.

**Recommended: an app password.**

1. In the Pi-hole web interface, go to **Settings → Web interface / API**, switch to
   **Expert** mode and use **Configure app password**. Copy it into `PIHOLE_PASSWORD`.
2. App passwords are read-only by default. Allow them to change config by setting
   `webserver.api.app_sudo` to `true` (**Settings → All settings → Webserver and API**), or run:

   ```sh
   pihole-FTL --config webserver.api.app_sudo true
   ```

Without `app_sudo`, adding or removing records fails with HTTP 403 and the log says so.

**Alternative:** the main web password. Two-factor authentication is not supported; with 2FA
enabled, use an app password (app passwords skip 2FA).

### Optional settings

- `PIHOLE_RESTART_DNS=true` additionally needs `webserver.api.allow_destructive=true`.
- Pi-hole allows `webserver.api.max_sessions` (default 16) concurrent API sessions. The tool
  uses one session per pass and always logs out, so it does not use up seats.

## First run

Do a dry run first and check the planned records:

```sh
docker run --rm --env-file ./omada2pihole.env -e DRY_RUN=true \
  ghcr.io/filipegalo/omada2pihole:latest --once
```

Each planned change is logged as `msg=plan op=add record="10.0.0.11 laptop.lan"`. When the
list looks right, drop `DRY_RUN`.
