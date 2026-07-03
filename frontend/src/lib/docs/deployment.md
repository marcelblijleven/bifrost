# Production deployment (LXC)

This guide covers running Bifrost on a Linux host using an LXC container, systemd services, PostgreSQL, and nginx with TLS. The same steps apply to any Debian/Ubuntu environment - the LXC wrapper is optional.

## Architecture

```
Internet
    │
    ▼
nginx :443 (TLS)
    ├── /webhooks/*  ──►  Go backend   :8080  (internal)
    ├── /healthz     ──►  Go backend   :8080  (internal)
    └── /*           ──►  SvelteKit    :3000  (internal)
                              │
                              └── API_URL ──►  Go backend :8080
```

The browser only ever talks to nginx. The SvelteKit Node.js server handles SSR and proxies all API calls (including SSE) to the Go backend using `API_URL`.

---

## 1. Create the LXC container

```bash
lxc launch ubuntu:24.04 bifrost
lxc shell bifrost
```

Inside the container, create a dedicated user:

```bash
useradd -r -s /bin/false -m -d /var/lib/bifrost bifrost
```

---

## 2. Install dependencies

```bash
apt update && apt install -y \
  nginx certbot python3-certbot-nginx \
  postgresql postgresql-contrib \
  curl

# Node.js 22 LTS
curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt install -y nodejs

# pnpm
npm install -g pnpm
```

---

## 3. PostgreSQL

```bash
systemctl enable --now postgresql

sudo -u postgres psql <<'SQL'
CREATE USER bifrost WITH PASSWORD 'changeme';
CREATE DATABASE bifrost OWNER bifrost;
SQL
```

The connection string for `.env` will be:

```
DATABASE_URL=postgres://bifrost:changeme@localhost:5432/bifrost?sslmode=disable
```

---

## 4. Build Bifrost

Build on your development machine (or inside the container if you install Go):

```bash
# Backend
GOOS=linux GOARCH=amd64 go build -o bifrost ./cmd/bifrost

# Frontend
cd frontend
pnpm install --frozen-lockfile
pnpm build
# Produces: frontend/build/
```

Copy to the container:

```bash
# From your dev machine
lxc file push bifrost bifrost/usr/local/bin/bifrost
lxc file push -r frontend/build bifrost/var/lib/bifrost/frontend-build
```

Set ownership:

```bash
# Inside container
chown bifrost:bifrost /usr/local/bin/bifrost
chmod 755 /usr/local/bin/bifrost
chown -R bifrost:bifrost /var/lib/bifrost
```

---

## 5. Environment file

Create `/etc/bifrost/env`:

```bash
mkdir -p /etc/bifrost
cat > /etc/bifrost/env <<'EOF'
HTTP_ADDR=127.0.0.1:8080
DATABASE_URL=postgres://bifrost:changeme@localhost:5432/bifrost?sslmode=disable
JWT_SECRET=<openssl rand -hex 32>
API_KEY=<openssl rand -hex 32>
PUBLIC_URL=https://bifrost.example.com

# GitHub - use one of:
GITHUB_TOKEN=ghp_...

# OR GitHub App:
# GITHUB_APP_ID=123456
# GITHUB_INSTALLATION_ID=78901234
# GITHUB_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
EOF

chmod 640 /etc/bifrost/env
chown root:bifrost /etc/bifrost/env
```

For the SvelteKit server, create `/etc/bifrost/frontend.env`:

```bash
cat > /etc/bifrost/frontend.env <<'EOF'
PORT=3000
HOST=127.0.0.1
ORIGIN=https://bifrost.example.com
API_URL=http://127.0.0.1:8080
EOF

chmod 640 /etc/bifrost/frontend.env
chown root:bifrost /etc/bifrost/frontend.env
```

`ORIGIN` must match the public URL exactly - SvelteKit uses it for CSRF protection.

---

## 6. systemd services

### Go backend - `/etc/systemd/system/bifrost.service`

```ini
[Unit]
Description=Bifrost release orchestrator (API)
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=bifrost
Group=bifrost
EnvironmentFile=/etc/bifrost/env
ExecStart=/usr/local/bin/bifrost
Restart=on-failure
RestartSec=5s

# Hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/bifrost

[Install]
WantedBy=multi-user.target
```

### SvelteKit frontend - `/etc/systemd/system/bifrost-web.service`

```ini
[Unit]
Description=Bifrost frontend (SvelteKit)
After=network.target bifrost.service
Wants=bifrost.service

[Service]
Type=simple
User=bifrost
Group=bifrost
WorkingDirectory=/var/lib/bifrost/frontend-build
EnvironmentFile=/etc/bifrost/frontend.env
ExecStart=/usr/bin/node index.js
Restart=on-failure
RestartSec=5s

NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

Enable and start both:

```bash
systemctl daemon-reload
systemctl enable --now bifrost bifrost-web
systemctl status bifrost bifrost-web
```

---

## 7. nginx

### Initial HTTP config - `/etc/nginx/sites-available/bifrost`

```nginx
server {
    listen 80;
    server_name bifrost.example.com;

    # GitHub webhooks go directly to the Go backend
    location /webhooks/ {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }

    # Health check (for load balancers / uptime monitors)
    location /healthz {
        proxy_pass http://127.0.0.1:8080;
    }

    # Everything else → SvelteKit (handles SSR, auth, SSE proxy)
    location / {
        proxy_pass             http://127.0.0.1:3000;
        proxy_set_header       Host $host;
        proxy_set_header       X-Real-IP $remote_addr;
        proxy_set_header       X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header       X-Forwarded-Proto $scheme;

        # Required for SSE (run progress streaming)
        proxy_buffering        off;
        proxy_cache            off;
        proxy_read_timeout     3600s;
    }
}
```

```bash
ln -s /etc/nginx/sites-available/bifrost /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```

### TLS with Let's Encrypt

```bash
certbot --nginx -d bifrost.example.com
```

Certbot rewrites the nginx config to redirect HTTP → HTTPS and add the certificate. Auto-renewal is handled by the `certbot.timer` systemd unit.

### Restrict `/metrics` to internal access only

Add inside the `server` block after certbot runs:

```nginx
location /metrics {
    allow 10.0.0.0/8;     # internal monitoring network
    allow 127.0.0.1;
    deny all;
    proxy_pass http://127.0.0.1:8080;
}
```

---

## 8. LXC network (expose to the internet)

On the **host**, forward ports into the container:

```bash
# Replace 10.x.x.x with your container's IP (lxc list to find it)
lxc config device add bifrost port80  proxy listen=tcp:0.0.0.0:80  connect=tcp:10.x.x.x:80
lxc config device add bifrost port443 proxy listen=tcp:0.0.0.0:443 connect=tcp:10.x.x.x:443
```

If the host has a firewall:

```bash
ufw allow 80/tcp
ufw allow 443/tcp
```

---

## 9. First run

Navigate to `https://bifrost.example.com`. Because no users exist yet, you are redirected to `/setup` to create the admin account. After that, the setup endpoint is permanently disabled.

You can also create users via the CLI from your dev machine:

```bash
BIFROST_URL=https://bifrost.example.com \
BIFROST_TOKEN=<api-key-from-env> \
  bifrost-cli users create --email you@example.com --password secret
```

---

## 10. Upgrading

```bash
# Build new binary and frontend on dev machine, then:
lxc file push bifrost bifrost/usr/local/bin/bifrost
lxc file push -r frontend/build bifrost/var/lib/bifrost/frontend-build

# Inside container
systemctl restart bifrost bifrost-web
```

Migrations run automatically on startup - no manual steps needed.

---

## Monitoring

| What | Where |
|---|---|
| Application logs | `journalctl -u bifrost -f` |
| Frontend logs | `journalctl -u bifrost-web -f` |
| Prometheus metrics | `https://bifrost.example.com/metrics` (internal only) |
| Health check | `https://bifrost.example.com/healthz` |

Exposed Prometheus metrics:

| Metric | Description |
|---|---|
| `bifrost_pipeline_runs_total` | Total runs by status |
| `bifrost_pipeline_run_duration_seconds` | Run duration histogram by status |
| `bifrost_running_runs` | Currently executing runs (gauge) |

---

## Database backups

```bash
# Daily backup via cron (inside container, or via lxc exec)
crontab -u bifrost -e
# 0 3 * * * pg_dump -U bifrost bifrost | gzip > /var/lib/bifrost/backups/bifrost-$(date +\%Y\%m\%d).sql.gz
```

Keep at least 7 days of backups offsite.
