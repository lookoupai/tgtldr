# TGTLDR

[中文版](README.md)

TGTLDR (Telegram Too Long, Don't Read) is a single-user, self-hosted Telegram group monitoring and daily AI summary system.

This project exists because many Telegram groups are large, noisy communities that can produce thousands of messages per day. Sometimes you only want the latest useful signals without spending a lot of time reading through the chat. TGTLDR can push the previous day's key conclusions to you at a fixed time every day.

![TGTLDR home screen](docs/images/home-en.png)

## Features

- Monitor Telegram groups you have joined and store messages in a local database
- Configure daily summary time, prompts, filters, and summary model per group
- Summarize by channel, or group summaries by AI-detected topics or configured topic groups
- Read multilingual channels and configure the default summary output language or override it per group
- Generate group summaries through an OpenAI-compatible API
- Read summaries in the web app, with optional Telegram Bot delivery; long Bot messages are split automatically
- Manually trigger summaries, view historical summaries, and retry failed Bot deliveries
- Define custom knowledge spaces to extract long-lived structured facts and user profiles from messages
- Query knowledge facts from the web app or Bot commands, such as demands, supplies, skills, events, or your own schema types
- Complete first-time Telegram, OpenAI, and group setup through the web wizard

## Requirements

- Docker and Docker Compose, recommended for running the system
- Telegram `api_id` and `api_hash`, available from [my.telegram.org/apps](https://my.telegram.org/apps)
- OpenAI-compatible Base URL, API Key, and model name
- Optional: Telegram Bot Token for sending summaries back to Telegram

## Local Startup

### Recommended: use prebuilt images

```bash
cp .env.example .env
docker compose up -d
```

If `TGTLDR_MASTER_KEY` is not explicitly set, the system generates a random master key on first startup and persists it in the app container data volume.

To pull a specific image version before startup:

```bash
export TGTLDR_IMAGE_NAMESPACE=fr0der1c
export TGTLDR_IMAGE_TAG=latest
docker compose up -d
```

If port `3000` is already occupied on the host, or if you want to listen on all network interfaces instead of localhost only, override these values in `.env`:

```bash
cp .env.example .env
# Edit .env and set the values you need:
# TGTLDR_HOST_BIND=0.0.0.0
# TGTLDR_HOST_WEB_PORT=13000
docker compose up -d
```

Where:

- `TGTLDR_HOST_BIND=127.0.0.1` listens only on the local machine, suitable for default local use
- `TGTLDR_HOST_BIND=0.0.0.0` listens on all network interfaces, suitable for servers or NAS deployments

`TGTLDR_MASTER_KEY` is the local data encryption master key. It encrypts stored Telegram sessions, OpenAI API Keys, and Bot Tokens. It is never sent to external services. By default, this key is saved in the app data volume at `/var/lib/tgtldr/master.key`. If you delete that volume, previously stored sensitive data can no longer be decrypted.

After startup, open:

- Web app: `http://localhost:${TGTLDR_HOST_WEB_PORT}` (default `http://localhost:3000`)

On first visit, follow the setup wizard to configure the access password, Telegram, OpenAI, and group summary settings.

## Operations

Check container status:

```bash
docker compose ps
```

View logs:

```bash
docker compose logs -f app
docker compose logs -f web
docker compose logs -f postgres
```

Check backend health:

```bash
curl http://127.0.0.1:3000/api/health
```

Upgrade to the latest prebuilt images:

```bash
docker compose pull
docker compose up -d
```

If the web app behaves unexpectedly after an upgrade, check the `app` logs first to confirm database migrations and backend startup completed successfully.

## Summaries and Knowledge Spaces

Each group can configure its own summary behavior:

- `Channel`: the default mode, producing one summary for the previous day's messages
- `Topics`: AI groups discussions by topic; you can also configure topic names and descriptions per group
- `Summary output language`: built-in options include Chinese, English, Russian, and Arabic; you can also enter a custom language name, and groups inherit the global default when left blank
- Bot delivery respects Telegram's single-message limit and automatically splits content above 4096 visible characters

Knowledge spaces maintain long-lived information instead of one-off summaries. Each space has its own JSON schema, extraction instructions, target groups, confidence threshold, and retention period. This makes the feature reusable for demand/supply channels, hiring, skill profiles, event signups, project leads, or other custom scenarios. TGTLDR creates a general-purpose template when no knowledge spaces exist, and you can edit or replace it with your own rules.

Facts with the same knowledge space, chat, type, title, and subject user are merged automatically. TGTLDR keeps the earliest first-seen time, refreshes the latest last-seen time, and combines source messages as evidence.

If Bot delivery is enabled and a target chat is configured, you can query the knowledge base from that chat:

```text
/knowledge <keyword>
/type <fact_type> <keyword>
/fact <fact_type> <keyword>
/facts <fact_type> <keyword>
/demand <keyword>
/supply <keyword>
/who <keyword>
/ask <natural-language question>
/expire <fact_id>
/forget <fact_id>
/restore <fact_id>
/update <natural language>
/confirm <code>
/cancel
```

Use `/type` for custom schema types, such as `/type hiring remote` or `/type skill rust`. `/fact` and `/facts` are aliases for the same query form.
Query results include fact IDs. Use `/expire` to mark a fact expired, `/forget` to dismiss it, and `/restore` to reactivate it.
`/ask who understands crypto trading` parses a natural-language question into query filters, searches the knowledge base, then generates an evidence-based answer with fact IDs. If there is not enough evidence, it says so directly. You can also use natural-language maintenance commands such as `/update Alice no longer needs Gmail`; the system previews matching facts and requires `/confirm <code>` before updating them. Web actions, Bot commands, natural-language updates, and automatic status updates are recorded as maintenance events so you can audit the previous status, next status, and source.

The Bot only responds in the configured target chat, so local knowledge is not sent to unauthorized conversations.

### Developer: local Docker build

If you need to modify code and rebuild images locally, use the development override:

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build
```

### Manual development startup

If you already started the system with Docker, you do not need this section. Manual startup is intended for development and debugging, and requires you to prepare PostgreSQL, Go, and Node.js yourself.

Start the backend:

```bash
cd app
export TGTLDR_DATABASE_URL='postgres://postgres:postgres@localhost:5432/tgtldr?sslmode=disable'
export TGTLDR_MASTER_KEY_FILE="$HOME/.tgtldr/master.key"
export TGTLDR_MASTER_KEY='replace with a value generated by openssl rand -base64 32'
go run ./cmd/server
```

Start the frontend:

```bash
cd web
npm install
TGTLDR_INTERNAL_API_BASE_URL=http://127.0.0.1:8080 npm run dev
```

## Security Notes

- `TGTLDR_MASTER_KEY` encrypts stored Telegram sessions, OpenAI API Keys, and Bot Tokens.
- If `TGTLDR_MASTER_KEY` is not explicitly set, the system generates a random key and persists it to `/var/lib/tgtldr/master.key`.
- Keep this key or the corresponding data volume safe. If it is lost, secrets and Telegram sessions already stored in the database cannot be decrypted.
- Deploy only on localhost or a trusted private network by default. If exposing the service publicly, first complete access password setup and place it behind a trusted reverse proxy.

## Reverse Proxy Deployment

If you plan to expose the service through a reverse proxy, configure these values in `.env` first:

```env
TGTLDR_HOST_BIND=0.0.0.0
TGTLDR_WEB_ORIGIN=https://tgtldr.example.com
TGTLDR_HOST_WEB_PORT=13000
```

Where:

- `TGTLDR_HOST_BIND`: lets the container listen on all server network interfaces
- `TGTLDR_WEB_ORIGIN`: the public URL users actually visit
- `TGTLDR_HOST_WEB_PORT`: the local port your reverse proxy forwards to

Then start the service:

```bash
cp .env.example .env
# Edit .env
docker compose up -d
```

The reverse proxy only needs to forward traffic to the local port represented by `TGTLDR_HOST_WEB_PORT`.

Nginx example, assuming `TGTLDR_HOST_WEB_PORT=13000`:

```nginx
server {
    listen 80;
    server_name tgtldr.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name tgtldr.example.com;

    ssl_certificate     /path/to/fullchain.pem;
    ssl_certificate_key /path/to/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:13000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## Image Publishing

- The default `docker-compose.yml` is for regular users and uses prebuilt images directly.
- `docker-compose.dev.yml` is for developers and keeps the local build workflow.
- GitHub Actions automatically builds and pushes these images when `main` or `v*` tags are pushed:
  - `fr0der1c/tgtldr-app`
  - `fr0der1c/tgtldr-web`

## License

This project uses the [PolyForm Noncommercial License 1.0.0](LICENSE).

You may use, fork, modify, and distribute this project for noncommercial purposes. Commercial use requires separate authorization from the author.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Product flow and implementation plan](docs/PRODUCT_FLOW.md)
- [Knowledge space configuration and examples](docs/knowledge-spaces.md)
