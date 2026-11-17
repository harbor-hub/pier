# Pier

Local HTTP/HTTPS reverse proxy with a built-in DNS stub. Opens `http://my-app.pier` in the browser even when the backend IP is not reachable from the host (Docker Desktop, WSL2 from Windows).

## How it works

Pier reads host records from a JSON file or URL. The DNS stub answers `127.0.0.1` for your domains, the browser connects to Pier, and Pier forwards the request to the record's target.

```
browser → my-app.pier → 127.0.0.1    (Pier DNS stub)
        → Pier :80 / :443
        → 172.17.0.2:80              (target from records)
```

## Installation

### 1. Create records

```bash
mkdir -p ~/.pier
tee ~/.pier/records.json <<EOF
{"records": [
  {"host": "my-app.pier", "target": "172.17.0.2"},
  {"host": "dev.pier", "target": "host.docker.internal", "labels": {"pier.upstream": "5173"}}
]}
EOF
```

Pier re-reads the file every 5 seconds; no restart needed.

### 2. Start Pier

> Port 53 is often occupied on the host, so the DNS stub is published on port `5153`.

```bash
docker run -d \
  --name pier \
  --restart unless-stopped \
  --log-opt max-size=1m \
  --log-opt max-file=3 \
  --add-host host.docker.internal:host-gateway \
  -p 127.0.0.1:80:80 \
  -p 127.0.0.1:5153:53/udp \
  -v ~/.pier:/etc/pier:ro \
  ghcr.io/harbor-hub/pier:latest \
    --dns-listen :53 \
    --http-listen :80 \
    --source /etc/pier/records.json \
    --verbose
```

Pier must be able to reach the targets. A dev server on the host (`host.docker.internal`) must listen on `0.0.0.0`, not `localhost` (e.g. `vite --host`).

#### Reaching containers

Docker does not route between networks, so Pier must share a network with every container IP in the records. Use one shared network:

```bash
docker network create shared
docker network connect shared pier
```

Pier stays on `bridge` too, so it reaches containers on either network. Attach your containers to `shared` as well, and make sure the records use their IP on a network Pier is on. On `shared`, a records URL can use a container name: `--source http://<container>/<path>`.

On Linux and WSL2 you can instead run Pier with `--network host` (drop the `-p` options): the host reaches every Docker network.

### 3. Configure DNS

Add one entry per domain configured in `--dns-domain` (default `pier`).

#### Linux (systemd-resolved)

```conf
# /etc/systemd/resolved.conf.d/pier.conf
[Resolve]
DNS=127.0.0.1:5153
Domains=~pier
```

```sh
sudo systemctl restart systemd-resolved
```

#### macOS

```bash
sudo mkdir -p /etc/resolver
sudo tee /etc/resolver/pier <<EOF
nameserver 127.0.0.1
port 5153
EOF
```

#### Windows

NRPT rules cannot use a custom port, so publish the stub on port 53 (`-p 127.0.0.1:53:53/udp`), then in PowerShell as administrator:

```powershell
Add-DnsClientNrptRule -Namespace ".pier" -NameServer "127.0.0.1"
```

## Usage

Each record maps a host to a target (IP or hostname):

```json
{"records": [
  {"host": "my-app.pier", "target": "172.17.0.2"}
]}
```

Requests go to port 80 of the target. Use labels to change that:

| Label | Example | Description |
|---|---|---|
| `pier.upstream` | `3000` | Backend for HTTP and HTTPS clients |
| `pier.upstream.http` | `http://:3000` | Backend for HTTP clients only |
| `pier.upstream.https` | `https://:8443` | Backend for HTTPS clients only |
| `pier.redirect` | `https` | Redirect HTTP clients to HTTPS |

Backend values:

| Value | Description |
|---|---|
| `3000` | Same as `http://:3000` |
| `https://:8443` | Backend speaks HTTPS; add `?insecure=true` for a self-signed certificate |
| `tls://:8443` | TLS passthrough: the client gets the backend's own certificate (HTTPS clients only) |
| `tls://:8443?proxyprotocol=2` | Passthrough with a PROXY protocol header (`1` or `2`) carrying the client address |

### HTTPS

Pier terminates HTTPS with its own certificate and forwards plain HTTP to the backend with `X-Forwarded-Proto: https`. Hosts with a `tls://` backend are forwarded untouched instead; they need no certificate in Pier.

Create a certificate with [mkcert](https://github.com/FiloSottile/mkcert) and add to the `docker run` command:

```bash
mkcert -install
mkcert -cert-file ~/.pier/cert.pem -key-file ~/.pier/key.pem "*.pier"
```

```bash
  -p 127.0.0.1:443:443 \
  ...
    --https-listen :443 \
    --https-cert /etc/pier/cert.pem \
    --https-key /etc/pier/key.pem
```

Without `--https-cert` and `--https-key`, `--https-listen` serves `tls://` hosts only.

### Records from a URL

`--source` also accepts a URL returning the same JSON. Use `--source-token` to send `Authorization: Bearer <token>`.

### TCP ports

`pier tcp` forwards raw TCP (MySQL, Redis, Postgres...) from a local port to a target, without records. The target name is resolved on every connection, so a container recreated with a new IP keeps working.

```bash
pier tcp <listen>=<host>:<port> [<listen>=<host>:<port> ...]

pier tcp 5432=192.168.1.50:5432
pier tcp --resolver 127.0.0.1:5053 3306=mysql.my-app.lh:3306 6379=redis.my-app.lh:6379
```

`<listen>` is a port (all addresses) or `<addr>:<port>`, e.g. `127.0.0.1:3306`. `--resolver` sends name lookups to a specific DNS server instead of the system resolver.

In Docker every mapping needs its own `-p`; publish on `127.0.0.1` so the ports are not exposed to the network:

```bash
docker run -d \
  --name pier-tcp \
  --restart unless-stopped \
  --network shared \
  -p 127.0.0.1:3306:3306 \
  -p 127.0.0.1:6379:6379 \
  ghcr.io/harbor-hub/pier:latest \
    tcp \
      --resolver <dns-container>:53 \
      3306=mysql.my-app.lh:3306 6379=redis.my-app.lh:6379
```

| Flag | Variable | Default | Description |
|---|---|---|---|
| `--resolver` | `PIER_TCP_RESOLVER` | system | DNS server for target names, `host[:port]` (port `53` by default) |
| `--dial-timeout` | `PIER_TCP_DIAL_TIMEOUT` | `5s` | Timeout for connecting to a target |
| `--verbose` | `PIER_VERBOSE` | `false` | Log every connection |

## Configuration

All options can be set via flags or environment variables. Empty listen address = disabled.

| Flag | Variable | Default | Description |
|---|---|---|---|
| `--dns-listen` | `PIER_DNS_LISTEN` | | DNS stub listen address |
| `--dns-domain` | `PIER_DNS_DOMAIN` | `pier` | Domains the DNS stub answers for (comma-separated) |
| `--dns-stub` | `PIER_DNS_STUB` | `127.0.0.1` | IP address the DNS stub answers with |
| `--http-listen` | `PIER_HTTP_LISTEN` | | HTTP listen address |
| `--https-listen` | `PIER_HTTPS_LISTEN` | | HTTPS listen address |
| `--https-cert` | `PIER_HTTPS_CERT` | | TLS certificate file (not needed for `tls://` hosts) |
| `--https-key` | `PIER_HTTPS_KEY` | | TLS key file |
| `--source` | `PIER_SOURCE` | | Records file or URL |
| `--source-token` | `PIER_SOURCE_TOKEN` | | Bearer token for a URL source |
| `--source-token-file` | `PIER_SOURCE_TOKEN_FILE` | | File containing the bearer token |
| `--source-refresh` | `PIER_SOURCE_REFRESH` | `5s` | Records refresh interval |
| `--verbose` | `PIER_VERBOSE` | `false` | Log DNS queries and requests |
