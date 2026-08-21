# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

This repository currently contains only a product requirements document (`docs/prd/HomeStreamer.md`) — there is no source code, build tooling, tests, or dependency manifest yet. There are no build/lint/test commands to document until implementation begins. When code is added, update this file with the actual commands (build, lint, test, run a single test, Docker commands).

## What this product is

Personal TV (a.k.a. "HomeStreamer") is an open-source, self-hosted platform that turns a user's local media library into configurable, scheduled virtual TV channels with an electronic program guide (EPG) — the experience of "pick a channel and watch what's on" rather than browsing a file library.

Full requirements live in `docs/prd/HomeStreamer.md`. Read it before starting implementation work — it is the source of truth for scope. Key points future work must respect:

## Core domain model

The scheduling engine is the product's core differentiator and must stay independent of the UI:

```
Channel → Schedule → Program → Media
```

- **Media** — a playable item (movie, episode, video) sourced from the user's local filesystem. Represented abstractly (`title`, `duration`, `type`, `source`, `metadata`) so the scheduler never depends on where media came from.
- **Channel** — a virtual TV station with its own independent schedule; users create/rename/delete/enable/disable channels.
- **Program** — an entry that will play on a channel, referencing media (or, in the future, a collection/playlist/rule). Its end time is derived from media duration.
- **Schedule** — determines what plays on a channel and when. MVP supports sequential playlists only (explicit start times, end times computed from duration); rule-based/random/recurring scheduling is a future capability, not MVP.

At any point the system must be able to answer, per channel: what's playing now, when did it start/end, and what plays next (and after that).

## Architectural layering

Maintain clear separation between these layers; the UI must consume the API rather than reach into scheduling/media internals directly:

```
Media Management → Scheduling → Channel State → Playback → API → Clients
```

- Media source access should go through an abstraction (local filesystem is the only MVP source) so future sources (Plex, Jellyfin, network streams, etc.) can be added as adapters without touching the scheduler.
- The API is the contract for the browser-based MVP client and any future clients (mobile, smart TV, desktop) — design endpoints for media sources, media items, channels, programs, schedules, and current playback/EPG state.
- Docker Compose is the intended local deployment method from the start.

## MVP scope discipline

This PRD explicitly restricts scope — do not add these unless a future spec explicitly requests them: streaming-service integrations (Netflix/Hulu/YouTube/etc.), cloud media storage, torrent/piracy-site functionality, DRM circumvention, media downloading, social/sharing features, mobile/smart-TV apps, recommendation systems, AI-generated schedules, advanced auth, payments/billing, or cloud infrastructure. Authentication is optional for the local-only MVP but the architecture shouldn't preclude adding it later.

When implementation details are ambiguous: check `docs/prd/HomeStreamer.md` first, prefer the simplest implementation consistent with it, don't expand scope without approval, and surface architecturally-significant decisions before implementing them rather than guessing.
