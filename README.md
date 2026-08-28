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
