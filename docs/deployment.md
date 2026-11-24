# Deployment

The tool is a single static binary, published as a multi-arch container image
(`linux/amd64`, `linux/arm64`, `linux/arm/v7`) and as release archives.

```
ghcr.io/filipegalo/omada2pihole:<version>
```

The image is based on `gcr.io/distroless/static` and runs as a non-root user (UID 65532).

All examples read the configuration from an env file; start from
[`examples/omada2pihole.env.example`](../examples/omada2pihole.env.example) and keep
the real file out of version control.

## Docker

```sh
# single pass
docker run --rm --env-file ./omada2pihole.env ghcr.io/filipegalo/omada2pihole:latest --once

# long-running, every 5 minutes
docker run -d --name omada2pihole --restart unless-stopped \
  --env-file ./omada2pihole.env \
  ghcr.io/filipegalo/omada2pihole:latest --interval 5m
```

To trust a private CA, mount it:

```sh
docker run --rm --env-file ./omada2pihole.env \
  -v "$PWD/ca.pem:/ca.pem:ro" -e OMADA_CA_FILE=/ca.pem \
  ghcr.io/filipegalo/omada2pihole:latest --once
```

## Docker Compose

```yaml
services:
  omada2pihole:
    image: ghcr.io/filipegalo/omada2pihole:latest
    command: ["--interval", "5m"]
    env_file: ./omada2pihole.env
    restart: unless-stopped
    read_only: true
    security_opt: ["no-new-privileges:true"]
    cap_drop: ["ALL"]
```

## Cron

With Docker:

```cron
*/5 * * * * docker run --rm --env-file /etc/omada2pihole.env ghcr.io/filipegalo/omada2pihole:latest --once >> /var/log/omada2pihole.log 2>&1
```

With the release binary (load the env file first; `set -a` exports every variable):

```cron
*/5 * * * * set -a; . /etc/omada2pihole.env; /usr/local/bin/omada2pihole --once >> /var/log/omada2pihole.log 2>&1
```

## Kubernetes (Helm)

The Helm chart is published as an OCI artifact next to the image:

```
oci://ghcr.io/filipegalo/charts/omada2pihole
```

By default it installs a CronJob that runs `--once` every 5 minutes (`concurrencyPolicy: Forbid`,
non-root, read-only root filesystem, all capabilities dropped, no service account token).
With `mode: deployment` it runs a single long-lived pod that loops instead.

Write a values file:

```yaml
# values.yaml
config:
  omada:
    url: https://omada.example.lan:8043
    id: 0123456789abcdef0123456789abcdef
    site: Default
  pihole:
    url: https://pi.hole
  dryRun: true          # check the plan first, then set to false
credentials:
  existingSecret: omada2pihole
```

Keep the secrets out of the values file by creating the Secret yourself. It must hold
`OMADA_CLIENT_ID`, `OMADA_CLIENT_SECRET` and `PIHOLE_PASSWORD`:

```sh
kubectl create namespace omada2pihole
kubectl -n omada2pihole create secret generic omada2pihole \
  --from-literal=OMADA_CLIENT_ID=... \
  --from-literal=OMADA_CLIENT_SECRET=... \
  --from-literal=PIHOLE_PASSWORD=...

helm install omada2pihole oci://ghcr.io/filipegalo/charts/omada2pihole \
  --namespace omada2pihole -f values.yaml
```

The Secret is loaded with `envFrom`, so it may also hold any other setting; you can create it
from a complete env file with `--from-env-file`. Values set under `config` take precedence over
the Secret, and settings left empty or `false` in the chart are not passed to the container.

Alternatively, set `credentials.omadaClientId`, `credentials.omadaClientSecret` and
`credentials.piholePassword` and the chart creates the Secret.

Run a pass right away and read its log:

```sh
kubectl -n omada2pihole create job --from=cronjob/omada2pihole manual-1
kubectl -n omada2pihole logs -f job/manual-1
```

### Chart values

| Value | Default | Description |
|---|---|---|
| `mode` | `cronjob` | `cronjob` (one pass per schedule) or `deployment` (long-running loop). |
| `cronjob.schedule` | `*/5 * * * *` | Cron schedule. `cronjob.timeZone`, `suspend`, history limits, `backoffLimit` and `activeDeadlineSeconds` (240) are also available. |
| `deployment.interval` | `5m` | Loop interval in `deployment` mode (minimum `30s`). |
| `config.omada.url`, `config.omada.id` | | Required unless provided by `credentials.existingSecret`. |
| `config.omada.site` | | Site name (application default `Default`). |
| `config.pihole.url` | | Required unless provided by `credentials.existingSecret`. |
| `config.omada.tlsSkipVerify`, `config.pihole.tlsSkipVerify` | `false` | Skip TLS verification. |
| `config.omada.caFile`, `config.pihole.caFile` | | Path of a PEM CA file in the container; mount it with `extraVolumes`. |
| `config.pihole.restartDns` | `false` | `PIHOLE_RESTART_DNS`. |
| `config.domainSuffix` | | Record suffix (application default `lan`). |
| `config.dryRun`, `config.allowMassDelete` | `false` | `DRY_RUN`, `ALLOW_MASS_DELETE`. |
| `config.logLevel` | | `info` or `debug`. |
| `credentials.existingSecret` | | Use this Secret instead of creating one. |
| `image.repository`, `image.tag`, `image.digest` | chart `appVersion` | Image to run; `digest` wins over `tag`. |
| `extraEnv`, `extraVolumes`, `extraVolumeMounts` | `[]` | Additional env vars and volumes. |
| `resources`, `podSecurityContext`, `securityContext`, `nodeSelector`, `tolerations`, `affinity`, `podAnnotations`, `podLabels` | hardened defaults | Usual pod settings. |

See [`values.yaml`](../charts/omada2pihole/values.yaml) for all values.

Trusting a private CA from a ConfigMap:

```yaml
config:
  pihole:
    caFile: /etc/omada2pihole/ca.pem
extraVolumes:
  - name: ca
    configMap:
      name: my-ca          # holds the key ca.pem
extraVolumeMounts:
  - name: ca
    mountPath: /etc/omada2pihole
    readOnly: true
```

## Building from source

```sh
make build    # ./bin/omada2pihole
make docker   # omada2pihole:dev image
```
