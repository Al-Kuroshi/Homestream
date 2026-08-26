# Personal TV — Product Requirements Document

**Status:** Draft
**Version:** 1.1 (canonical)
**Product type:** Open-source, self-hosted personal television platform

## 1. Overview

Personal TV turns a personal video library into a fully configurable television service. Users point the platform at their movies, TV episodes, personal videos, and recordings, then build virtual channels that decide what plays and when. Instead of picking a video from a list, a user turns on a channel and watches whatever is currently scheduled — the way a real TV network works, but programmed entirely by them.

The long-term plan is two products built on one core: a free, open-source, self-hosted version for people who want to run it themselves, and (later) a hosted version for people who'd rather not manage their own infrastructure.

## 2. Vision

The goal is to let anyone build their own personal television network — take a collection of movies, shows, episodes, and recordings and turn it into channels like *Action Movies*, *Comedy*, *90s Cartoons*, *Anime*, or *Family TV*, each running independently on its own schedule. The core experience should always come back to one idea:

> Turn on a channel and watch what's currently playing, rather than deciding what to watch every time.

## 3. Core Concepts

The platform is built from four ideas that build on each other: **Media → Channels → Programs → Schedule.**

**Media** is any playable content available to the platform — movies, TV episodes, short videos, personal recordings, or other compatible video files. Media may eventually come from several kinds of sources, but the initial implementation focuses on media the user already controls (their own files).

**Channels** are virtual television stations. A user can create any number of them — `Movies`, `Comedy`, `Anime`, `Cartoons`, `Family`, and so on — and each one is configured, scheduled, and enabled/disabled independently.

**Programs** are entries scheduled to play on a channel. A program usually points at a specific movie or episode, but may eventually point at a playlist, a collection of media, or an automated rule that picks something dynamically.

**Schedule** is what actually decides when each program plays on a channel, for example:

```
18:00  Movie
20:00  TV Episode
20:30  TV Episode
21:00  Movie
```

The platform calculates each program's end time automatically from its media duration, and from that can always determine what's currently playing on any channel.

## 4. Target Users

The primary users are individuals with a personal media collection — local storage or a NAS, comfort with self-hosted software, and a desire for a more passive, television-like way of watching what they already own. Households that want customized channels built from a shared media collection are a natural secondary audience. Further out, the target expands to people who want the same experience without managing their own hardware or storage — customers of a future hosted version.

## 5. Product Principles

**Television first.** The product should feel like a television service, not a media browser with a scheduler bolted on. The core loop is: choose a channel → watch the current program → see what's next.

**User control.** Users keep control over their media and how it's programmed. Using the core product should never require surrendering that control.

**Self-hosted first.** The open-source version must be fully usable on a user's own hardware, with Docker as a primary, first-class deployment method.

**Extensible media sources.** The scheduler should never be tightly coupled to "local files" as a concept. Media access is built around a source abstraction, so additional sources — NAS, network storage, user-controlled cloud storage, Plex, Jellyfin, YouTube, and other legally supported services — can be added later without touching the scheduling core.

## 6. MVP Scope

The MVP delivers a complete personal-television experience built entirely on user-controlled, self-hosted media.

**Media library.** Configure one or more media directories, scan them, and discover compatible video files. Browse, search, and filter the discovered media, view its basic metadata, and rescan or remove a media source as the library changes. The platform references files where they already live rather than duplicating them.

**Channel management.** Create, rename, delete, and reorder channels; enable or disable them individually; configure a description and, where supported, artwork. Each channel maintains its own independent schedule.

**Scheduling.** The MVP supports straightforward sequential scheduling:

```
18:00 → Movie A
20:05 → Episode 1
20:35 → Episode 2
21:05 → Movie B
```

End times are calculated automatically from media duration. Users can add or remove scheduled media, reorder programs, change scheduling times, and view upcoming programs.

**Playback.** Users select a channel, watch whatever is currently scheduled, and switch channels freely. The interface shows the current program's title and progress, what plays next, and standard playback controls. The system determines the correct playback position from the schedule itself — if a program started at 20:00 and the user joins at 20:25, playback begins at approximately the corresponding point in that program, not from the beginning.

**Electronic Program Guide (EPG).** A television-style guide shows every channel's current and upcoming programs with start and end times, for example:

```
             18:00       19:00       20:00       21:00

Movies       Movie A                 Movie B
Comedy                   Show A      Show B
Anime        Episode 1   Episode 2   Movie
```

**User interface.** Five primary areas: **TV** (the video player, current channel/program, progress, next-up, and channel selection), **Guide** (the EPG), **Media Library** (browse/search/filter/select media for programming), **Channels** (create/edit/delete channels and their schedules), and **Settings** (media sources, playback, application, and system configuration).

**API.** The platform exposes an API covering media sources, media items, channels, programs, schedules, current playback state, and EPG data, designed so that future clients (mobile, desktop, smart TV, other media players) can consume it independently of the web interface.

**Docker.** The application deploys via Docker, with Docker Compose as the primary path, without requiring complex infrastructure. Local and NAS-backed storage are first-class, and initial configuration stays simple.

## 7. Future Scheduling Capabilities

None of the following are required for the MVP, but the architecture should not preclude them: recurring programs ("every day at 20:00 → Movie"), random selection ("21:00 → random movie from Action"), collection-based picks ("22:00 → random episode from Comedy Shows"), rotation rules ("don't repeat the same movie within 30 days"), time-of-day programming blocks (e.g. Kids in the morning, Movies at night), and special one-off events ("Friday 21:00 → Movie Night").

**Recurring programs now have a concrete design:** `docs/design/2026-08-26-recurring-slot-scheduling-design.md` scopes weekly-recurring, duration-sized slot scheduling (approved, not yet implemented). TV-series/episode auto-advance and YouTube-sourced slots are explicitly deferred further, as their own future specs, within that same document.

## 8. Storage Models

Three storage models exist across the product's lifetime, only the first of which is in scope for the MVP:

- **Self-hosted storage** — media stays on the user's own computer or NAS; this is the primary model for the open-source MVP.
- **Bring-your-own cloud storage** — a future hosted version may let users connect their own S3, R2, B2, GCS, or similar bucket, with the platform managing the television experience while the user keeps control of the underlying storage.
- **Managed storage** — a future commercial service may sell storage directly (e.g. 500 GB–10 TB tiers). Outside the MVP.

## 9. External and Streaming Integrations

External services (YouTube, Plex, Jellyfin, other user-controlled storage) may eventually become optional integrations, implemented as modular media sources — the core scheduler should never contain service-specific logic.

Commercial streaming platforms (Netflix, Hulu, and similar) require special handling: the platform must never circumvent DRM, download protected streams, rehost protected content, bypass authentication, or circumvent access restrictions. Where a service's terms and APIs allow it, a future integration might offer metadata, search, deep links, playback handoff, or account linking — but the exact functionality depends entirely on what each service permits. None of this is in scope for the MVP.

## 10. Copyright and User Content

The open-source application provides infrastructure for users' own media libraries — it is not a piracy tool. It must not include piracy search engines, torrent search or download functionality, piracy-site scraping, DRM circumvention or removal, a centralized library of copyrighted content, or any redistribution of media between users. Users remain responsible for having appropriate rights to the media they use. If a future hosted service ever stores user media directly, appropriate legal policies must be established before that service launches.

## 11. Reliability and Performance

The platform should keep working correctly through the normal failures of a long-running self-hosted service: application restarts, missing or deleted media files, unreadable or invalid media, and temporary playback failures. A single unavailable media item should degrade gracefully, not take down the whole channel.

It should also scale reasonably with a real personal collection — media scanning shouldn't require loading the entire library into memory, and should avoid reprocessing unchanged files where possible. The scheduler itself should stay lightweight enough to manage multiple channels simultaneously without strain.

## 12. Authentication and Users

Authentication is not required for the initial local-only MVP if it would add unnecessary complexity, but the architecture must not preclude adding it later. Future versions may add multiple users, accounts, profiles, permissions, household accounts, parental controls, and shared channels.

## 13. Open-Source Business Model

The core self-hosted application stays open source and remains a genuinely functional product on its own — it should never be intentionally crippled to push users toward a paid tier. Future revenue is expected to come from things layered on top: a hosted version of Personal TV, managed storage, premium integrations, remote access, cloud backups, managed updates, enterprise features, and premium support.

## 14. MVP Non-Goals

The following are explicitly out of scope for the MVP and should not be implemented without a future spec that explicitly calls for them: Netflix/Hulu/YouTube integration, managed cloud storage, cloud backups, payments or subscriptions, a public channel marketplace or sharing, social features, mobile or smart-TV apps, advanced recommendation systems, AI-generated programming, DRM-related functionality, torrent or piracy functionality, and advanced multi-user authentication.

## 15. MVP User Journey

```
Install Personal TV
        ↓
Configure media source
        ↓
Scan media
        ↓
Media library populated
        ↓
Create channel
        ↓
Select media
        ↓
Create schedule
        ↓
Open channel
        ↓
Watch current program
        ↓
View upcoming programs
```

A first-time user should be able to reach a working channel without complicated configuration.

## 16. MVP Success Criteria

The MVP succeeds when a user can: install the application; configure local media storage; scan and discover their videos; browse their media library; create multiple channels; add media to those channels; build a schedule for each; have the platform automatically determine current and upcoming programs; watch a channel and switch between channels; view an electronic program guide; restart the application without losing configuration; and run the whole thing with Docker.

The central test is simple:

> **A user can take their existing video collection and turn it into a functioning personalized television network.**

## 17. Long-Term Vision

```
                    PERSONAL TV
                         │
          ┌──────────────┼──────────────┐
          │              │              │
      Self-Hosted     Hosted         Integrations
          │              │              │
       Local/NAS     Cloud Storage    YouTube
          │              │              Plex
          │              │              Jellyfin
          │              │              Other sources
          └──────────────┼──────────────┘
                         │
                         ▼
                  Personal Channels
                         │
                         ▼
                  Television Experience
```

The core idea should stay simple even as the product grows: give users the ability to create and operate their own television channels from media they control. Getting that experience right comes before expanding into a broader media ecosystem.
