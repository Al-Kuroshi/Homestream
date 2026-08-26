# Personal TV — Local Development & Testing

This is the practical companion to `CLAUDE.md` (which covers repo conventions and AI-assistant guidance): how to get the app running on your machine, run the test suites, and manually verify the pieces that automated tests can't reach (playback in a real browser).

## Prerequisites

- **Go 1.22+** (this repo currently builds with Go 1.25).
- **Node.js** (any version compatible with Vite 8 / the `web/package.json` deps — Node 20+ recommended).
- **`ffmpeg` and `ffprobe` on `PATH`.** Required to build a correct mental model of, and to run the test suite for, the media scanner and playback backend — several tests generate short synthetic videos and transcode/probe them for real, not mocked. Also required at runtime for HLS transcoding.

Verify these are available:

```bash
go version
node --version
ffmpeg -version
ffprobe -version
```

## First-time setup

```bash
git clone <this repo>
cd HomeStreamProject
go mod download
cd web && npm install && cd ..
```

## Running the app

There are two ways to run it, depending on what you're doing.

### Dev mode (frontend + backend, hot reload)

Two processes, in two terminals:

```bash
# Terminal 1 — backend
go run ./cmd/personaltv

# Terminal 2 — frontend dev server (proxies /api/* to the backend on :8080)
cd web && npm run dev
```

Open the URL Vite prints (typically `http://localhost:5173`). Frontend changes hot-reload; backend changes require restarting `go run`.

### Production mode (single binary, what actually ships)

**Build order matters.** `web/embed.go` embeds whatever is in `web/dist` at Go build time. Build the frontend first, then the backend:

```bash
cd web
npm run build
cd ..
go build ./...
./personaltv
```

or, to build and run in one step:

```bash
cd web && npm run build && cd .. && go run ./cmd/personaltv
```

Open `http://localhost:8080` (or whatever `PERSONALTV_PORT` is set to) — this single process serves both the API and the UI.

If you skip the `npm run build` step, `go build`/`go run` still succeeds, but embeds only the tracked placeholder in `web/dist`, not the real UI — you'll get a working API and a blank/placeholder frontend.

### Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PERSONALTV_DB_PATH` | `personaltv.db` | SQLite database file path |
| `PERSONALTV_PORT` | `8080` | HTTP listen port |
| `PERSONALTV_SESSIONS_DIR` | a subdirectory under the OS temp dir | Where HLS transcode sessions write their playlist/segment files |

## Running the test suites

### Backend (Go)

```bash
go build ./...                  # build
go vet ./...                    # lint
gofmt -l .                      # format check (empty output = clean)
go test ./...                   # test
go test ./... -race             # test with race detector (run before merging/finishing a branch)
```

Run a single package or test pattern:

```bash
go test ./internal/mediastore/... -run TestScanner -v
```

### Frontend (from `web/`)

```bash
cd web
npx tsc -b        # type-check
npm run lint      # oxlint
npm test          # vitest, one-shot
npm run test:watch # vitest, watch mode
npm run build     # type-check + production build into web/dist
```

The full verification sequence used before merging a frontend change:

```bash
cd web && npx tsc -b && npm run lint && npm test && npm run build
```

## Manual browser verification (TV/player screen)

Unit and component tests mock the `<video>` element, `hls.js`, and network calls — they can prove the code is wired correctly, but nothing in the automated suite has ever actually decoded a video frame. Do this manual pass after any change that touches playback (`internal/playback`, `internal/api/playback_handlers.go`, or `web/src/api/playback.ts`/`web/src/screens/TVScreen.tsx`/`web/src/components/VideoPlayer.tsx`), and periodically otherwise.

**Setup**, if you don't already have a channel with something scheduled to be playing right now:

1. Run the app in production mode (above).
2. **Settings** → add a media source pointing at a folder with a video file → rescan.
3. **Channels** → create a channel → open its schedule → add the scanned media item with a start time a few minutes in the past.

**Checklist**, at `/tv`:

| Check | What to look for |
|---|---|
| Direct playback | An h264/aac/mp4 file plays automatically, seeked to roughly the right point (e.g. a program that started 3 minutes ago should start playback ~3 minutes in, not from the beginning) |
| Tap-to-play fallback | If the browser blocks autoplay, a "▶ Tap to play" button appears over the video instead of a silently stuck frame |
| Now-playing overlay | Title + progress bar show on load, fade out after a few seconds of no mouse movement, and reappear on mouse movement |
| Channel switching | The prev/next arrows and the "☰" channel-list overlay (top-right) switch channels and start playing the new one |
| Off-air / interstitial | A channel with nothing currently scheduled shows "Nothing scheduled right now" with a live countdown if something's coming up later, or a static "nothing else scheduled" message if not |
| HLS transcoding | See below |

**HLS mode** needs a file that fails the direct-play compatibility check (`internal/playback/compat.go`: anything other than h264 video + aac/mp3 audio inside an mp4-family container). If you don't have one handy, generate one:

```bash
ffmpeg -f lavfi -i testsrc=duration=30:size=640x480:rate=25 \
       -f lavfi -i sine=frequency=440:duration=30 \
       -c:v libx264 -c:a aac -f matroska test.mkv
```

(The `.mkv` container alone is enough to force the HLS path, even with otherwise-compatible codecs inside it.) Schedule it on a channel and confirm: video starts playing (this exercises a real `ffmpeg` transcode plus `hls.js` in the browser), no console errors about `hls.js`/MediaSource Extensions, and playback starts near the beginning of the transcoded session rather than jumping around.

## Troubleshooting

- **`go test` fails on `internal/mediastore`/`internal/playback`/`internal/integration` with subprocess errors** — `ffmpeg`/`ffprobe` aren't on `PATH`, or are too old to support the flags used (`-force_key_frames`, `-hls_playlist_type`).
- **`go test ./web/...` skips with a message about `web/dist` not being built** — expected on a fresh clone; run `npm run build` in `web/` first if you need that test to actually run.
- **Frontend `npm run lint` reports warnings (not errors) on `playback.ts`/`TVScreen.tsx`** — three known warnings (two false-positive `oxlint` rule triggers in `playback.ts`, one benign `set-state-in-effect` in `TVScreen.tsx`) are tracked in `docs/PROGRESS.md`; `npm run lint`'s exit code stays `0`, they're not build-blocking.
