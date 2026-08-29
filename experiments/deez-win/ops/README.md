# Deploying deez.win

Target: `46.101.104.158` (`dice`), alongside codraw.app, dopa.win, godloop.ai
and friends. **That box runs live services** — everything here is scoped so a
leaked CI credential cannot reach them.

## Shape

CI builds the binary (the server has no Go toolchain), uploads it, and calls one
script. Same build → scp → `systemctl restart` flow as the other services, just
automated.

| Piece | Where |
|---|---|
| `deez-win.service` | `/etc/systemd/system/` — runs as `www-data` on `127.0.0.1:8087` |
| `deez.win.nginx.conf` | `/etc/nginx/sites-available/deez.win` → symlink into `sites-enabled` |
| `deez-win-deploy` | `/usr/local/bin/` — swap, restart, roll back on failure |
| `deez-win-ssh-guard` | `/usr/local/bin/` — forced command for the CI key |

Port **8087** was free at setup time (8080, 8082–8084, 8090, 8091 were taken).

## One-time server setup

```bash
# 1. app directory
mkdir -p /opt/deez-win && chown www-data:www-data /opt/deez-win

# 2. scripts
install -m 755 deez-win-deploy /usr/local/bin/deez-win-deploy
install -m 755 deez-win-ssh-guard /usr/local/bin/deez-win-ssh-guard

# 3. service
install -m 644 deez-win.service /etc/systemd/system/deez-win.service
systemctl daemon-reload && systemctl enable deez-win

# 4. nginx (after the DNS A record resolves to this host)
install -m 644 deez.win.nginx.conf /etc/nginx/sites-available/deez.win
ln -sf /etc/nginx/sites-available/deez.win /etc/nginx/sites-enabled/deez.win
nginx -t && systemctl reload nginx
certbot --nginx -d deez.win -d www.deez.win
```

## The CI key

Generate a key **used for nothing else**:

```bash
ssh-keygen -t ed25519 -f deez-win-ci -C "ci-deez-win" -N ""
```

On the server, add the public half with the guard as a forced command:

```
command="/usr/local/bin/deez-win-ssh-guard",restrict ssh-ed25519 AAAA... ci-deez-win
```

`restrict` turns off pty and all forwarding. The guard then permits exactly two
operations: writing `/opt/deez-win/deezwin.new`, and running the deploy script.
An interactive shell, a different path, or any other command is refused. Test it:

```bash
ssh -i deez-win-ci root@46.101.104.158            # must be refused
ssh -i deez-win-ci root@46.101.104.158 whoami     # must be refused
```

## GitHub secrets

Repository → Settings → Secrets → Actions:

| Secret | Value |
|---|---|
| `DEPLOY_SSH_KEY` | the **private** half of `deez-win-ci` |
| `DEPLOY_HOST` | `46.101.104.158` |
| `DEPLOY_USER` | the account the key is installed under |
| `DEPLOY_KNOWN_HOSTS` | `46.101.104.158 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIN1rcIsH8x2jOIlROedzngpexkItxPCTg64fIfLDN3Pl` |

The host key is pinned rather than auto-accepted, so a hijacked DNS answer can't
capture the deploy key. Fingerprint: `SHA256:F3dX+abxIKy52RAX0FEIdiLDmvZ7kcvrFqTzXaOCpAU`.

The `deploy` job targets a `production` environment — add required reviewers
there if you want a human approving each release.

## Runtime secrets

The service reads `/opt/deez-win/env` (optional, `EnvironmentFile=-`):

```
CALA_API_KEY=...
```

Without it the game runs on offline fixtures. `chmod 600`, owned by `www-data`.
It is deliberately not in git.

## DNS

`deez.win` is on Cloudflare (`saanvi`/`darwin.ns.cloudflare.com`). Point the A
record at `46.101.104.158`. Certbot's HTTP-01 challenge needs the record
**unproxied** (grey cloud) — turn the proxy back on afterwards if you want it.

## Rollback

```bash
ssh root@46.101.104.158
cp /opt/deez-win/deezwin.prev /opt/deez-win/deezwin && systemctl restart deez-win
```

The deploy script keeps the previous binary and rolls back automatically if the
service fails to come up, so this is only for a deploy that starts but misbehaves.
