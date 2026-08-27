# syntax=docker/dockerfile:1

# ---- Stage 1: frontend build ----
FROM node:22-slim AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: Go build ----
FROM golang:1.25-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/personaltv ./cmd/personaltv

# ---- Stage 3: runtime ----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ffmpeg \
      curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /usr/sbin/nologin personaltv \
    && mkdir -p /data \
    && chown personaltv:personaltv /data

COPY --from=builder /out/personaltv /usr/local/bin/personaltv

WORKDIR /data
USER personaltv
EXPOSE 8080
ENV PERSONALTV_DB_PATH=/data/personaltv.db
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["personaltv"]
