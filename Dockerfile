# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache ca-certificates git

WORKDIR /src

# Cache dependency downloads.
COPY go.mod go.sum ./
RUN go mod download

# Build the binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /spp-event-bot .

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Run as non-root.
RUN addgroup -S bot && adduser -S bot -G bot

# Create data directory for state persistence.
RUN mkdir -p /data && chown bot:bot /data

COPY --from=builder /spp-event-bot /usr/local/bin/spp-event-bot

USER bot

VOLUME ["/data"]

ENTRYPOINT ["spp-event-bot"]
