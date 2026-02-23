# Deploy Guide: spp-event-bot

Step-by-step instructions for building, testing, and deploying the SPP event bot.

## Prerequisites

- Go 1.22+ (for local development)
- Docker (for building the container image)
- A running Tuwunel Matrix homeserver
- An iCal (.ics) feed URL
- A Matrix room to post events to

## Local Development

### Build and run locally

```bash
# Download dependencies
go mod tidy

# Run tests
go test -v ./...

# Build
go build -o spp-event-bot .

# Run (set required env vars)
export FEED_URL="https://example.com/events.ics"
export MATRIX_HOMESERVER="https://matrix.sparklingpinkpandas.com"
export MATRIX_ACCESS_TOKEN="syt_your_token_here"
export MATRIX_ROOM_ID="!your_room_id:sparklingpinkpandas.com"
export DATA_DIR="./data"
export POLL_INTERVAL="60"  # 1 minute for testing

./spp-event-bot
```

### Run tests

```bash
# All tests
go test -v ./...

# With race detection
go test -race ./...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # view in browser
```

### Test with a local iCal file

You can serve a test `.ics` file locally:

```bash
# Create a test file
cat > /tmp/test.ics << 'EOF'
BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:test-001@local
DTSTART:20260301T100000Z
DTEND:20260301T120000Z
SUMMARY:Test Ride
DESCRIPTION:A test event to verify the bot works.
LOCATION:City Park
URL:https://example.com/test
END:VEVENT
END:VCALENDAR
EOF

# Serve it
python3 -m http.server 8080 --directory /tmp &

# Point the bot at it
export FEED_URL="http://localhost:8080/test.ics"
```

## Build Container Image

### Build locally

```bash
docker build -t ghcr.io/YOUR_ORG/spp-event-bot:latest .
```

### Push to registry

```bash
# Login to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u YOUR_USERNAME --password-stdin

# Push
docker push ghcr.io/YOUR_ORG/spp-event-bot:latest
```

### Automated builds

The `build.yml` GitHub Actions workflow automatically builds and pushes to GHCR on every push to `main`. To set it up:

1. Push the repo to GitHub:
   ```bash
   gh repo create YOUR_ORG/spp-event-bot --public --source=. --push
   ```
2. The `GITHUB_TOKEN` is automatically available — no secrets to configure
3. The image will be at `ghcr.io/YOUR_ORG/spp-event-bot:latest`

## Deploy to Kubernetes

### 1. Create a bot user on the Matrix server

In the Tuwunel admin room (`#admins:sparklingpinkpandas.com`):
```
!admin users create @event-bot:sparklingpinkpandas.com YourSecurePassword
```

### 2. Get an access token

```bash
curl -s -X POST \
  https://matrix.sparklingpinkpandas.com/_matrix/client/v3/login \
  -H 'Content-Type: application/json' \
  -d '{
    "type": "m.login.password",
    "identifier": {"type": "m.id.user", "user": "event-bot"},
    "password": "YourSecurePassword"
  }' | jq .access_token
```

Save this token — you'll need it for the Kubernetes secret.

### 3. Find the room ID

In Element, open the `#rides` room, go to **Settings > Advanced** and copy the **Internal room ID** (starts with `!`).

### 4. Create the Kubernetes secret

From the `matrix-k8s` repo:

```bash
cp secrets/examples/event-bot-secrets.example.yaml secrets/event-bot-secrets.yaml
# Edit: set MATRIX_ACCESS_TOKEN to the token from step 2
kubectl apply -f secrets/event-bot-secrets.yaml
```

### 5. Configure the bot

Edit `event-bot/configmap.yaml` in the `matrix-k8s` repo:

```yaml
data:
  FEED_URL: "https://your-actual-feed-url.com/events.ics"
  POLL_INTERVAL: "900"
  MATRIX_HOMESERVER: "http://tuwunel.matrix.svc.cluster.local:6167"
  MATRIX_ROOM_ID: "!your_room_id:sparklingpinkpandas.com"
```

### 6. Update the deployment image

Edit `event-bot/deployment.yaml` in the `matrix-k8s` repo:

```yaml
containers:
  - name: event-bot
    image: ghcr.io/YOUR_ORG/spp-event-bot:latest  # your actual image
```

### 7. Apply

```bash
kubectl apply -f event-bot/configmap.yaml
kubectl apply -f event-bot/deployment.yaml
```

### 8. Verify

```bash
# Check pod is running
kubectl get pods -n matrix -l app.kubernetes.io/name=spp-event-bot

# Watch logs
kubectl logs -n matrix -l app.kubernetes.io/name=spp-event-bot -f
```

You should see:
```json
{"level":"INFO","msg":"starting spp-event-bot","feed_url":"https://...","poll_interval":"15m0s",...}
{"level":"INFO","msg":"matrix client initialized"}
{"level":"INFO","msg":"polling feed","url":"https://..."}
{"level":"INFO","msg":"fetched events from feed","count":5}
```

## Configuration Reference

| Env Variable | Required | Default | Description |
|---|---|---|---|
| `FEED_URL` | Yes | — | URL of the iCal (.ics) feed |
| `MATRIX_HOMESERVER` | Yes | — | Matrix homeserver URL |
| `MATRIX_ACCESS_TOKEN` | Yes | — | Bot's access token |
| `MATRIX_ROOM_ID` | Yes | — | Room ID to post events to |
| `POLL_INTERVAL` | No | `900` | Seconds between feed polls |
| `DATA_DIR` | No | `/data` | Directory for persisting state |

## How It Works

1. On startup, the bot checks `/data/last-seen.txt` for the last event timestamp it processed
2. It fetches the iCal feed from `FEED_URL`
3. Parses all `VEVENT` components
4. Filters to events with a `DTSTART` after the last-seen timestamp
5. Posts each new event to the Matrix room as a formatted message with title, date/time range, location, and description
6. Updates the last-seen timestamp

### Message format

Events are posted as formatted Matrix messages:

> **[Saturday Morning Ride](https://example.com/rides/saturday)**
> **When:** Sat, 15 Feb 2026 4:00 PM - 7:00 PM
> **Location:** Coffee Shop Parking Lot
>
> Join us for a chill 30-mile ride through the hills.

## CI

Two GitHub Actions workflows:

- **`test.yml`** — Runs on every push/PR:
  - `go vet ./...`
  - `go test -v -race ./...`
  - `go build`
  - golangci-lint
  - hadolint (Dockerfile)

- **`build.yml`** — Runs on push to `main`:
  - Builds Docker image
  - Pushes to GHCR with `:latest` and `:sha` tags

## Troubleshooting

| Issue | Solution |
|---|---|
| Bot can't connect to homeserver | Check `MATRIX_HOMESERVER` URL. In-cluster use `http://tuwunel.matrix.svc.cluster.local:6167` |
| "MATRIX_ACCESS_TOKEN is required" | The secret isn't mounted. Check `kubectl describe pod` for secret mount errors |
| No events posted | Check logs for poll results. The bot only posts events *newer* than the last-seen timestamp. Delete `/data/last-seen.txt` to reprocess all events |
| Feed parse errors | Verify the feed URL returns valid iCal. Test with `curl -s YOUR_FEED_URL \| head -20` — should start with `BEGIN:VCALENDAR` |
| Duplicate posts | The bot tracks by timestamp, not UID. If the feed changes event dates, duplicates may occur. The `last-seen.txt` file can be manually edited |
