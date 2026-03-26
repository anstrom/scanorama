# ── Stage 1: Frontend build ───────────────────────────────────────────────────
# Builds the React/Vite frontend and places output in /internal/frontend/dist
# so Stage 2 can copy it into the Go source tree before embedding.
FROM node:22-alpine AS frontend-builder

WORKDIR /frontend

# Install deps first for better layer caching.
COPY frontend/package*.json ./
RUN npm ci --silent

# Build. vite.config.ts sets outDir to ../internal/frontend/dist which,
# relative to WORKDIR /frontend, resolves to /internal/frontend/dist.
COPY frontend/ ./
RUN npm run build


# ── Stage 2: Go build ─────────────────────────────────────────────────────────
# Compiles the Go binary with the frontend assets embedded via //go:embed.
FROM golang:1.24-alpine AS go-builder

RUN apk add --no-cache git

WORKDIR /build

# Cache Go module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy the full source tree (includes the placeholder internal/frontend/dist/).
COPY . .

# Overwrite the placeholder with the real frontend build from Stage 1.
COPY --from=frontend-builder /internal/frontend/dist ./internal/frontend/dist

# Build arguments for version injection (set by CI / docker buildx bake).
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN go build \
    -ldflags "-s -w \
              -X 'main.version=${VERSION}' \
              -X 'main.commit=${COMMIT}' \
              -X 'main.buildTime=${BUILD_TIME}'" \
    -o /scanorama \
    ./cmd/scanorama


# ── Stage 3: Runtime image ────────────────────────────────────────────────────
# Minimal Alpine image — only the binary and runtime deps (nmap).
FROM alpine:latest

RUN apk add --no-cache \
    ca-certificates \
    nmap

# Non-root user for security.
RUN addgroup -g 1000 scanorama && \
    adduser -D -s /bin/sh -u 1000 -G scanorama scanorama

WORKDIR /app

COPY --from=go-builder /scanorama /app/scanorama

RUN mkdir -p /app/config && \
    chown -R scanorama:scanorama /app

USER scanorama

EXPOSE 8080

ENTRYPOINT ["/app/scanorama"]
CMD ["daemon", "start", "--port", "8080"]
