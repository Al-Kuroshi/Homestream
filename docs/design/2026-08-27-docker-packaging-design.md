# Personal TV — Docker Packaging — Design Spec

**Status:** Approved by user in brainstorming session, ready for implementation planning.
**Supersedes:** nothing. Fulfills the Docker packaging placeholder called out in `docs/design/2026-08-21-personal-tv-design.md` §2, `CLAUDE.md`, and the PRD (`docs/prd/HomeStreamer.md` §5, §6, §16) — none of which this spec changes, only implements.
**Depends on:** the core backend, playback backend, and all frontend screens (Plans 1-5), all merged to `main`.

## 1. Scope

This is the last planned MVP piece: package the already-working single Go binary (with the embedded frontend) into a Docker image and a `docker-compose.yml`, per the deployment model already decided in `docs/design/2026-08-21-personal-tv-design.md` §2 ("Deployed as a single Docker container built from a single Go binary... media lives on storage the user already controls... mounted into the container via a Docker bind mount").

In scope:
- A multi-stage `Dockerfile` (frontend build → Go build → runtime image).
- A `docker-compose.yml` wiring up the media bind mount, a persistent volume for the SQLite database, and port mapping.
- A `.env.example` template for the user-specific values (media path, port).
- A top-level `README.md` with quick-start instructions — this repo currently has none.

Explicitly **out of scope** (confirmed during brainstorming):
- Publishing a pre-built image to a registry (Docker Hub, GHCR, etc.) or any CI pipeline to do so. The user builds locally via `docker compose build`.
- `docker buildx`/multi-arch manifest tooling. The Dockerfile is written to be architecture-agnostic (no hardcoded `GOARCH`), so `docker compose build` naturally produces a working image for whatever architecture the host's Docker daemon is (amd64 or arm64) — but a genuine multi-arch *manifest* (one tag serving both from a registry) is a distinct, unneeded concern without registry publishing.
- Verifying the image actually runs on real ARM hardware (Raspberry Pi, Synology/QNAP). No such hardware is available in this environment — this is an owed manual verification, the same class of gap as the browser-testing caveats already tracked in `docs/PROGRESS.md`.
- Any change to the application's own code, API, or behavior. This plan only packages what already exists.
- Cloud storage backends, multi-instance/orchestration (Kubernetes, Swarm), reverse-proxy/TLS termination guidance — all explicit MVP non-goals per the PRD, unaffected by this spec.

## 2. Dockerfile — three stages

**Stage 1 (`frontend`, `node` base):** `npm ci && npm run build` in `web/`, producing `web/dist`. Must complete before Stage 2, matching `CLAUDE.md`'s existing "build order matters" rule (`go:embed` embeds whatever's in `web/dist` at Go build time).

**Stage 2 (`builder`, `golang` base):** copies `web/dist` from Stage 1, then `CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/personaltv ./cmd/personaltv`. `CGO_ENABLED=0` is safe and free here — both of this module's dependencies (`modernc.org/sqlite`, `google/uuid`) are pure Go, so there's no C-toolchain/musl-compatibility concern, and it keeps the resulting binary a static executable. `TARGETOS`/`TARGETARCH` are Docker's automatically-populated build args — never hardcode `GOARCH=amd64` here, since that's what would silently break an arm64 build on a Pi/NAS.

**Stage 3 (runtime, `debian:bookworm-slim` base):**
- `apt-get install -y --no-install-recommends ffmpeg curl` (`ffmpeg`/`ffprobe` for transcoding — already a hard runtime requirement per `CLAUDE.md`; `curl` solely to give the `HEALTHCHECK` instruction something to call without adding a heavier dependency).
- Create a non-root user, create `/data` and `chown` it to that user *before* it's ever used as a volume mount point — this matters because Docker seeds a fresh named volume's initial contents/ownership from whatever already exists at that path in the image, so this ordering is what makes the named volume come up writable by the non-root user without a separate entrypoint chown step.
- `COPY --from=builder /out/personaltv /usr/local/bin/personaltv`
- `USER <nonroot>`, `EXPOSE 8080`, `HEALTHCHECK --interval=30s --timeout=3s CMD curl -f http://localhost:8080/healthz || exit 1` (reuses the existing `/healthz` endpoint — no new code needed), `ENTRYPOINT ["personaltv"]`.

## 3. `docker-compose.yml`

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

Decisions locked in during brainstorming:
- **Media mount is read-only (`:ro`).** The app only ever scans/reads/streams media, never writes to it — read-only is a free safety net against any future bug touching the user's real files.
- **Database persistence is a Docker **named volume** (`personaltv-data`), not a host bind-mount.** Docker-managed, no host-file-permission mismatches to troubleshoot. The user who wants direct host access to the SQLite file can still get it via `docker cp` or `docker compose exec`, but that's not the default path.
- **The HLS transcode-sessions directory is *not* given a volume at all.** It's ephemeral scratch space, regenerated per playback session (`PERSONALTV_SESSIONS_DIR`, already configurable, defaults to a temp directory) — nothing is lost by letting it live in the container's writable layer and disappear on removal.
- **`restart: unless-stopped`** — sensible default for something meant to run continuously on a home server.
- **Host port is configurable, container port is fixed at 8080** — `PERSONALTV_PORT` in `.env` only ever affects the host-side mapping; the app inside the container always listens on 8080, so there's exactly one moving part instead of two.

## 4. `.env.example`

```
# Host path to your media library (the folder containing your movies/shows).
MEDIA_PATH=/path/to/your/movies

# Port to expose the app on your host machine.
PERSONALTV_PORT=8080
```

The README must call out explicitly: once running, the media source you add *inside* the app (Settings → Sources) should point at `/media` — the path as seen *inside the container* — not the host path you set in `MEDIA_PATH`. This host-path/container-path split is the single most common point of confusion for a first-time self-hosted setup, and is worth a dedicated callout rather than assuming it's obvious.

## 5. `README.md`

Quick-start, in order: clone → `cp .env.example .env` → edit `MEDIA_PATH` → `docker compose up -d` → open `http://localhost:8080` → Settings → add a source at `/media` → rescan. Should also note the `ffmpeg`/`ffprobe`-on-`PATH` requirement is already handled inside the container image (nothing the user needs to install themselves when running via Docker — that requirement only applies to the non-Docker/local-dev workflow already documented in `docs/DEVELOPMENT.md`).

## 6. Testing

Docker is available and working in this environment (confirmed: `docker ps` reaches a live daemon), so this plan's verification is a **real, executable check**, not an owed manual pass:
1. `docker compose build` succeeds.
2. `docker compose up -d` — container reaches a healthy state (`docker compose ps` shows `healthy`), `http://localhost:8080` serves the real embedded UI.
3. Point `.env`'s `MEDIA_PATH` at a real media directory, add a source at `/media` through the running app, rescan, and confirm real files are discovered through the bind mount.
4. `docker compose down` then `docker compose up -d` again — confirm the channel/schedule data created in step 3 survived (the named volume persisted).
5. ARM (Raspberry Pi/NAS) execution stays genuinely unverified in this environment — tracked in `docs/PROGRESS.md` alongside the existing browser-testing and TV-screen caveats, not blocking.

## 7. Out of scope (this spec)

- Registry publishing / CI / buildx multi-arch manifests (§1)
- Real ARM hardware verification (§1, §6)
- Any application code/API/behavior change
- Reverse proxy, TLS, authentication, multi-instance orchestration
