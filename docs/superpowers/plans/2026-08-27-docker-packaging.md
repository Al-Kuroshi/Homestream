# Docker Packaging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Package the existing single Go binary (with its embedded frontend) into a Docker image and `docker-compose.yml`, plus a `.env.example` and a top-level `README.md`, so the app can be run via `docker compose up` per the deployment model already decided in this project's design docs.

**Architecture:** A 3-stage `Dockerfile` (build frontend → cross-compile the Go binary → package into a `debian:bookworm-slim` runtime image with `ffmpeg`/`curl`), a `docker-compose.yml` wiring a read-only media bind mount and a named volume for the SQLite database, and a `.env.example` for the two user-specific values (media path, host port). No application code changes.

**Tech Stack:** Docker, Docker Compose. Existing stack otherwise unchanged (Go 1.25, pure-Go SQLite driver — no cgo — React/Vite frontend, `ffmpeg`/`ffprobe`).

**Spec:** `docs/design/2026-08-27-docker-packaging-design.md`

## Global Constraints

- The runtime image's base is `debian:bookworm-slim` with `ffmpeg` and `curl` installed via `apt-get`.
- No hardcoded `GOARCH`/`GOOS` anywhere in the Dockerfile — use Docker's automatic `TARGETOS`/`TARGETARCH` build args, so `docker compose build` produces a correct image for whatever architecture the host's Docker daemon is.
- `CGO_ENABLED=0` for the Go build (safe: both of this module's dependencies, `modernc.org/sqlite` and `google/uuid`, are pure Go).
- The media volume is mounted read-only (`:ro`). The database lives in a Docker **named volume**, not a host bind-mount. The HLS transcode-sessions directory gets no volume at all — it's ephemeral by design.
- The container's internal port is always `8080`; only the host-side port mapping is configurable (`PERSONALTV_PORT` in `.env`).
- Out of scope: registry publishing, CI, `buildx`/multi-arch manifests, real ARM hardware verification, any application code change. See spec §1 for the full list.
- Docker is available and working in this environment (confirmed via `docker ps` reaching a live daemon) — verification in this plan means actually running `docker build`/`docker compose up`, not a manual/owed pass.

---

## Task 1: Dockerfile + `.dockerignore`

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Interfaces:**
- Produces: a buildable image tagged `personaltv:test` for this plan's own verification — no other task in this plan depends on Go/TS interfaces, since nothing here touches application code.

- [ ] **Step 1: Write `.dockerignore`**

```
.git
.claude
.superpowers
docs
web/node_modules
web/dist
*.db
*.db-wal
*.db-shm
personaltv
```

(`web/dist` is excluded from the build context because Stage 1 of the Dockerfile below rebuilds it fresh inside the image — the repo's tracked placeholder/gitignored local build artifacts should never be what ends up embedded in the shipped binary.)

- [ ] **Step 2: Write the Dockerfile**

```dockerfile
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

USER personaltv
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["personaltv"]
```

Notes for whoever implements this:
- The `mkdir -p /data && chown personaltv:personaltv /data` happens *before* `USER personaltv` and *before* any volume is mounted there — this ordering is what makes a fresh Docker named volume mounted at `/data` inherit the right ownership (Docker seeds a new named volume's initial contents/permissions from whatever already exists at that path in the image).
- Stage 2 copies the whole repo (`COPY . .`) after downloading Go modules separately first (for build-cache layering), then overwrites `./web/dist` with Stage 1's real build output — this ordering matters: if `COPY --from=frontend` happened before `COPY . .`, the second copy would silently stomp it back to whatever placeholder is tracked in git.

- [ ] **Step 3: Verify the image builds**

Run: `docker build -t personaltv:test .`
Expected: build succeeds through all 3 stages with no errors.

- [ ] **Step 4: Verify the binary actually runs and serves the embedded UI**

```bash
docker run --rm -d --name personaltv-test -p 18080:8080 personaltv:test
sleep 2
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:18080/healthz
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:18080/
docker logs personaltv-test
docker stop personaltv-test
```
Expected: both `curl` calls return `200`; logs show `Personal TV listening on :8080` with no errors; the root path response is the real embedded SPA (not a blank placeholder — you can confirm by piping the `/` response through `grep -o '<title>[^<]*'` and checking it's non-empty, or just checking the response body size is in the hundreds-of-KB range, matching the real bundle, not a near-empty placeholder page).

- [ ] **Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "feat: add multi-stage Dockerfile for the personaltv image"
```

---

## Task 2: `docker-compose.yml` + `.env.example`

**Files:**
- Create: `docker-compose.yml`
- Create: `.env.example`

**Interfaces:**
- Consumes: the `Dockerfile` from Task 1 (via `build: .`).
- Produces: the compose file Task 4's end-to-end verification runs against.

- [ ] **Step 1: Write `.env.example`**

```
# Host path to your media library (the folder containing your movies/shows).
MEDIA_PATH=/path/to/your/movies

# Port to expose the app on your host machine.
PERSONALTV_PORT=8080
```

- [ ] **Step 2: Write `docker-compose.yml`**

```yaml
services:
  personaltv:
    build: .
    restart: unless-stopped
    ports:
      - "${PERSONALTV_PORT:-8080}:8080"
    volumes:
      - personaltv-data:/data
      - ${MEDIA_PATH}:/media:ro
    environment:
      - PERSONALTV_DB_PATH=/data/personaltv.db
volumes:
  personaltv-data:
```

- [ ] **Step 3: Verify the compose file is syntactically valid**

```bash
cp .env.example .env
# Edit .env's MEDIA_PATH to any real existing directory on this machine for
# this syntax check (it doesn't need real media in it yet — Task 4 covers
# actually running it) — e.g.:
sed -i "s|MEDIA_PATH=.*|MEDIA_PATH=$(pwd)|" .env
docker compose config
```
Expected: `docker compose config` prints the fully-resolved config with no errors (confirms YAML validity, variable interpolation, and volume/port syntax are all correct) — it does not start any containers.

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml .env.example
git commit -m "feat: add docker-compose.yml and .env.example"
```

(Do not commit the `.env` file created for the syntax check in Step 3 — it's already outside version control by convention; confirm `git status` doesn't show it as untracked-and-about-to-be-added, and if it does, add `.env` to `.gitignore` before committing.)

---

## Task 3: `README.md`

**Files:**
- Create: `README.md`

**Interfaces:**
- None — pure documentation, no code interfaces.

- [ ] **Step 1: Write `README.md`**

```markdown
# Personal TV

Turn your local media library into your own set of scheduled TV channels — pick a channel and watch what's on, instead of browsing a file list. Self-hosted, single Docker container, your media stays on your own storage.

## Quick start

1. Clone this repo and `cd` into it.
2. Copy the environment template and edit it:
   ```bash
   cp .env.example .env
   ```
   Set `MEDIA_PATH` to the folder on your machine where your movies/shows live (e.g. `/home/you/Movies` or, on Windows via WSL, `/mnt/c/Users/you/Videos`).
3. Start it:
   ```bash
   docker compose up -d
   ```
4. Open `http://localhost:8080` (or whatever port you set `PERSONALTV_PORT` to).
5. Go to **Settings**, add a media source, and set its path to `/media` — **not** the host path you put in `.env`. Inside the container, your `MEDIA_PATH` folder is always mounted at `/media`, regardless of where it actually lives on your machine.
6. Rescan the source, then head to **Channels** to build a schedule, and **TV** to watch.

## Requirements

Just Docker and Docker Compose. `ffmpeg`/`ffprobe` are already installed inside the container image — you don't need them on your host unless you're running the app outside Docker for local development (see `docs/DEVELOPMENT.md` for that path).

## What this is

An open-source platform that scans your local video files and lets you build virtual, always-scheduled TV channels from them — see `docs/prd/HomeStreamer.md` for the full product spec, and `CLAUDE.md` for repo/architecture documentation if you're contributing.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add top-level README with Docker quick-start"
```

---

## Task 4: End-to-end verification

**Files:**
- None created — this task only runs and verifies the artifacts from Tasks 1-3. If it finds a real bug in any of them, fix that file directly as part of this task and note it in the report.

**Interfaces:**
- Consumes: `Dockerfile` (Task 1), `docker-compose.yml`/`.env.example` (Task 2).

This task proves the whole packaged deployment actually works, using a self-contained synthetic test video (matching this repo's existing convention of generating short synthetic videos with `ffmpeg` for tests, e.g. `internal/mediastore`/`internal/integration`) rather than depending on any specific real media library being present at a known path.

- [ ] **Step 1: Set up a synthetic media directory and `.env`**

```bash
mkdir -p /tmp/personaltv-docker-verify/movies
ffmpeg -y -f lavfi -i testsrc=duration=5:size=320x240:rate=10 \
       -f lavfi -i sine=frequency=440:duration=5 \
       -c:v libx264 -c:a aac -pix_fmt yuv420p \
       "/tmp/personaltv-docker-verify/movies/Test Movie.mp4"

cp .env.example .env
sed -i "s|MEDIA_PATH=.*|MEDIA_PATH=/tmp/personaltv-docker-verify/movies|" .env
```

- [ ] **Step 2: Build and start**

```bash
docker compose build
docker compose up -d
```

- [ ] **Step 3: Wait for healthy, confirm the real UI is served**

```bash
timeout 60 sh -c 'until [ "$(docker inspect -f "{{.State.Health.Status}}" $(docker compose ps -q personaltv))" = "healthy" ]; do sleep 2; done'
docker compose ps
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/
```
Expected: the wait loop exits before the 60s timeout (container reaches `healthy`); `docker compose ps` shows the service as `healthy`; the `curl` returns `200`.

- [ ] **Step 4: Confirm the bind-mounted media is actually scanned**

```bash
SOURCE_ID=$(curl -s -X POST http://localhost:8080/api/sources \
  -H "Content-Type: application/json" \
  -d '{"name":"Test","path":"/media"}' | grep -o '"id":[0-9]*' | grep -o '[0-9]*')
curl -s -X POST "http://localhost:8080/api/sources/$SOURCE_ID/scan"
curl -s http://localhost:8080/api/media
```
Expected: the scan succeeds, and `GET /api/media` shows exactly one item, titled from `Test Movie.mp4`, with `video_codec: "h264"`, `audio_codec: "aac"`, `invalid: false` — proving the container can see through the read-only bind mount and successfully shells out to `ffprobe` (confirming `ffmpeg`/`ffprobe` are genuinely present and working inside the runtime image, not just installed-but-broken).

- [ ] **Step 5: Confirm the database survives a restart (the named volume actually persists)**

```bash
CHANNEL_ID=$(curl -s -X POST http://localhost:8080/api/channels -H "Content-Type: application/json" -d '{"name":"Verify Channel"}' | grep -o '"id":[0-9]*' | grep -o '[0-9]*')

docker compose down
docker compose up -d
timeout 60 sh -c 'until [ "$(docker inspect -f "{{.State.Health.Status}}" $(docker compose ps -q personaltv))" = "healthy" ]; do sleep 2; done'

curl -s http://localhost:8080/api/channels
```
Expected: the channel created before `docker compose down` (id `$CHANNEL_ID`, name "Verify Channel") is still present in the response after the restart — proving the named volume persisted real data across a full container recreation, not just a process restart.

- [ ] **Step 6: Clean up**

```bash
docker compose down -v
rm -f .env
rm -rf /tmp/personaltv-docker-verify
```

(`down -v` removes the named volume too — this is a throwaway verification run, not a deployment you want to keep around. The `.env` file is removed since it was only created for this verification; a real user keeps their own `.env` permanently.)

- [ ] **Step 7: Report**

Write a short report to `/tmp/personaltv-docker-verify-report.md` (this file itself can be deleted after you've read it back to confirm — it doesn't need to be committed) covering: the exact output of each step above, and — critically — call out explicitly whether this ran on `linux/amd64` or `linux/arm64` (`docker version --format '{{.Server.Arch}}'`), since only the architecture actually available in this environment gets verified here; the other one remains an owed manual check per the spec's §1/§6.

If any step fails, fix the underlying file (`Dockerfile`/`docker-compose.yml`) directly, re-run from Step 2, and note what was wrong and how it was fixed.

No git commit for this task — it doesn't change any tracked file (unless Step 7 uncovers and fixes a real bug in an earlier task's files, in which case commit that fix with a clear message, e.g. `fix: <what was wrong> (found during Docker end-to-end verification)`).
