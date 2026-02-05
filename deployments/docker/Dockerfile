# =============================================================================
# Build Stage
# =============================================================================
FROM golang:1.24.0-alpine AS builder

# Build arguments
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy and download dependencies first (for better caching)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build server binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o /app/bin/server \
    ./cmd/server

# Build migrate binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/migrate \
    ./cmd/migrate

# =============================================================================
# Production Stage
# =============================================================================
FROM alpine:3.19 AS production

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/bin/server /app/server
COPY --from=builder /app/bin/migrate /app/migrate

# Copy migrations
COPY --from=builder /app/migrations /app/migrations

# Set ownership
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Environment defaults
ENV SERVER_HOST=0.0.0.0 \
    SERVER_HTTP_PORT=8080 \
    SERVER_ENV=production

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:${SERVER_HTTP_PORT}/health || exit 1

# Run the application
ENTRYPOINT ["/app/server"]

