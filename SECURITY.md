# Security policy

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately through
[GitHub private vulnerability reporting](https://github.com/filipegalo/omada2pihole/security/advisories/new).

Include the version (`omada2pihole --version`), what you found, and how to reproduce it.

## Supported versions

Only the latest release receives fixes.

## Scope

This tool handles an Omada OpenAPI client secret and a Pi-hole password. These are treated
as security bugs:

- a secret (client secret, access token, password, session id) appearing in logs, error
  messages, URLs or being sent to another host;
- the tool modifying or deleting Pi-hole `dns.hosts` entries it does not own.
