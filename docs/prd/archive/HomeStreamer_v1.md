# Personal TV — Product Requirements Document

**Status:** Draft
**Version:** 1.0
**Product Type:** Open-source, self-hosted personal TV/channel platform
**Primary Goal:** Turn a user's existing media library into configurable, scheduled television channels.

---

# 1. Product Overview

Personal TV is a self-hosted application that allows users to turn their own media library into a personalized television service.

Users provide the application access to locally stored media such as:

* Movies
* TV episodes
* Videos
* Personal recordings
* Other compatible video files

They can then create multiple virtual TV channels and configure what each channel plays and when.

Instead of manually selecting a video whenever they want to watch something, users can open a channel and watch whatever is currently scheduled, just like traditional television.

The core product is open source and self-hosted.

The user's media remains on their own hardware. The application provides the infrastructure for indexing, organizing, scheduling, and playing that media.

---

# 2. Product Vision

The product should make it possible for a user to create their own personal television network.

A user should be able to go from:

> "I have hundreds of movies and TV episodes sitting on my hard drive."

to:

> "I have 10 TV channels that continuously play my content according to schedules I created."

The experience should feel closer to operating a small television network than browsing a conventional media library.

---

# 3. Core Product Concept

The system consists of four primary concepts:

## 3.1 Media

A media item is a playable piece of content available to the application.

Examples:

* Movie
* TV episode
* Personal video
* Recorded program

Media is sourced from the user's own storage.

The application should not require users to upload their media to a centralized service.

---

## 3.2 Channels

A channel is a virtual television station.

A user can create any number of channels.

Examples:

* Channel 1 — Movies
* Channel 2 — Comedy
* Channel 3 — Anime
* Channel 4 — 90s Cartoons
* Channel 5 — Random TV
* Channel 6 — Family

Each channel has its own schedule and programming rules.

---

## 3.3 Programs

A program represents something that will play on a channel.

A program may reference:

* A specific movie
* A specific episode
* A collection of media
* A playlist
* A scheduling rule

The system determines when the program starts and ends based on its media duration and scheduling configuration.

---

## 3.4 Schedule

A schedule determines what plays on a channel and when.

Schedules should support both explicitly scheduled content and rules.

Examples:

```text
18:00 → Movie
20:00 → TV Show
20:30 → TV Show
21:00 → Movie
```

And eventually:

```text
18:00 → Random movie from "Action"
20:00 → Random episode from "Comedy Shows"
21:00 → Movie
```

The scheduling system is one of the core differentiating features of the product.

---

# 4. Target Users

## Primary User

A technically capable individual who has a personal collection of media and wants a more passive, television-like way of consuming it.

Typical characteristics:

* Has local storage or a NAS
* Has a collection of movies/TV shows/videos
* Is comfortable self-hosting applications
* Wants control over their media
* Enjoys customization

## Secondary Users

Small households that want personalized channels based on their existing media collection.

---

# 5. Product Principles

## 5.1 Local-first

The user's media should remain under the user's control.

The MVP should not require uploading media to a Personal TV cloud service.

---

## 5.2 Self-hostable

A user should be able to run the complete core application themselves.

Docker should be a first-class deployment method.

---

## 5.3 Open source

The core functionality should be available as open-source software.

The open-source version should be useful on its own and should not simply function as a restricted advertisement for a future paid service.

---

## 5.4 Source-agnostic media architecture

The scheduling engine should not care where a media item originated.

It should operate on an abstract media representation.

For example:

```text
MediaItem
├── title
├── duration
├── type
├── source
└── metadata
```

This allows future media-source integrations without coupling the scheduler to a particular source.

---

## 5.5 Television-first experience

The primary interaction should be:

> Pick a channel → watch what is currently playing.

The product should not feel like a normal file browser with a scheduler bolted onto it.

---

# 6. MVP Scope

The MVP should focus exclusively on the local/self-hosted experience.

## Included

### Media Library

Users can:

* Configure one or more local media directories
* Scan directories for media
* Detect supported video files
* View discovered media
* View media metadata
* Remove/re-scan media sources

The application should reference the user's files rather than duplicate them unnecessarily.

---

### Channel Management

Users can:

* Create channels
* Rename channels
* Delete channels
* Configure channel descriptions
* Configure channel ordering
* Enable/disable channels

Each channel should have its own independent programming schedule.

---

### Scheduling

Users can:

* Add media to a channel schedule
* Specify when programs should play
* Reorder scheduled programs
* Remove programs
* View a channel's schedule
* Modify schedules

The system should automatically determine program end times from media duration where appropriate.

---

### Playback

Users can:

* View available channels
* Switch between channels
* Watch the currently playing program
* See what is currently playing
* See what plays next
* View basic program information

The player should support standard video playback functionality.

---

### Electronic Program Guide

The application should provide an EPG-like interface showing:

* Channels
* Current program
* Upcoming programs
* Program start times
* Program end times

The EPG should make the application feel like a television service rather than a file browser.

---

### Basic Dashboard

The application should have a dashboard that provides:

* Channel list
* Current programming
* Upcoming programming
* Media library access
* Channel management
* Schedule management

---

### API

The application should expose an API for core operations.

The API should support, at minimum:

* Media sources
* Media items
* Channels
* Programs
* Schedules
* Playback/current-program information

The API should be designed so that a future mobile application, TV application, or external client can use it.

---

### Docker

The MVP should be deployable using Docker.

A basic self-hosted installation should be straightforward.

The project should provide a Docker Compose configuration for local deployment.

---

# 7. Scheduling Requirements

Scheduling is a core part of the product and should be designed independently from the UI.

The scheduler should understand:

```text
Channel
    ↓
Schedule
    ↓
Program
    ↓
Media
```

A channel should be able to determine:

1. What is playing now?
2. When did it start?
3. When will it end?
4. What plays next?
5. What will play after that?

---

## 7.1 Sequential Scheduling

The MVP should support sequential playlists.

Example:

```text
18:00 Movie A
20:05 Episode 1
20:35 Episode 2
21:05 Movie B
```

The system should calculate subsequent start times based on media duration.

---

## 7.2 Program Duration

A program based on a media item should normally inherit its duration from the media item.

The system must account for media duration when constructing the channel timeline.

---

## 7.3 Current Program

At any point in time, the system should be able to determine the current program for a channel.

Conceptually:

```text
Current Time
     ↓
Channel Schedule
     ↓
Program containing Current Time
     ↓
Current Media
     ↓
Playback Position
```

---

# 8. Future Scheduling Features

These are intentionally NOT part of the MVP but the architecture should avoid preventing them.

Potential future capabilities:

* Recurring programs
* Random selection
* Weighted random selection
* Playlists
* Categories
* "Play something from this collection"
* Minimum/maximum repeat intervals
* Time-of-day programming
* Day-of-week programming
* Seasonal schedules
* Fillers
* Commercial/intermission-style content
* Channel templates
* Automatic schedule generation
* Multiple schedule blocks
* Priority rules
* Content rotation

Example:

```text
Every Friday at 20:00
→ Select a random movie
→ Category: Action
→ Do not repeat for 30 days
```

---

# 9. Media Organization

The application should eventually understand relationships between media.

For example:

```text
TV Show
└── Season 1
    ├── Episode 1
    ├── Episode 2
    └── Episode 3
```

However, the MVP does not need to implement an advanced media metadata database.

The initial implementation should prioritize reliable media discovery and playback.

Metadata enrichment can be added later.

---

# 10. Media Sources

## MVP

The initial media source should be local filesystem storage.

Examples:

```text
/mnt/media/movies
/mnt/media/tv
/mnt/media/videos
```

The system should support multiple configured media directories.

---

## Future Sources

The architecture should permit additional source adapters.

Potential future integrations include:

* Plex
* Jellyfin
* YouTube
* Other legal media sources
* Network streams
* Public-domain media
* Podcasts/video feeds
* Other user-controlled storage

These are future capabilities and should NOT be implemented as part of the MVP.

---

# 11. External Streaming Services

Services such as YouTube, Netflix, Hulu, etc. are explicitly outside the MVP.

Future integrations must respect the relevant service's:

* APIs
* Terms of Service
* Playback restrictions
* Copyright requirements
* DRM requirements

The system must not attempt to circumvent DRM or access restrictions.

The architecture should allow future integrations through a media-source/plugin abstraction rather than embedding service-specific logic into the core scheduler.

---

# 12. Copyright and User Media

The product is intended to provide infrastructure for users' own media libraries.

The core system should not:

* Host a centralized library of copyrighted movies
* Provide piracy search functionality
* Download pirated content
* Scrape piracy websites
* Circumvent DRM
* Remove DRM
* Provide torrent-based piracy functionality
* Distribute copyrighted media between users

The self-hosted application should operate on media supplied by the user.

The product should not make assumptions about the legal status of individual media files.

For any future hosted/cloud service, legal requirements around user-uploaded content must be evaluated separately before implementation.

---

# 13. User Experience

The primary workflow should be simple.

## First Run

```text
Install
   ↓
Open application
   ↓
Configure media directory
   ↓
Scan media
   ↓
Create channel
   ↓
Add media
   ↓
Configure schedule
   ↓
Watch channel
```

A technically capable user should be able to reach a functioning channel quickly.

---

# 14. Main Application Areas

The UI should eventually contain:

### Home / TV

Shows:

* Current channel
* Current program
* Playback
* Next program
* Channel list

### Guide

Television-style EPG showing:

```text
Channel 1 | Movie A | Show A | Show B
Channel 2 | Show C  | Movie B | Movie C
Channel 3 | Movie D | Movie E | Show F
```

### Media Library

Allows users to:

* Browse media
* Search media
* Filter media
* View metadata
* Add media to programming

### Channels

Allows users to:

* Create channels
* Edit channels
* Delete channels
* Configure schedules

### Settings

Allows users to configure:

* Media sources
* Playback settings
* Application settings
* System settings

---

# 15. Non-Functional Requirements

## Performance

The application should be capable of handling a reasonably large personal media collection without requiring the entire library to be loaded into memory.

Media scanning should be incremental where practical.

---

## Reliability

A channel should continue playing scheduled content without requiring the user to manually start every program.

The scheduler should recover gracefully from:

* Missing files
* Deleted files
* Unreadable files
* Application restarts
* Temporary playback failures

---

## Extensibility

Core concepts should be separated:

```text
Media
Channels
Scheduling
Playback
API
UI
```

The scheduling system should not be tightly coupled to the web interface.

---

## Deployment

The project should support Docker-based deployment from the beginning.

The MVP should be usable without requiring a complicated cloud infrastructure.

---

# 16. Authentication

Authentication is NOT required for the first local-only MVP if it would significantly complicate initial development.

The architecture should not prevent authentication from being added later.

Future versions may support:

* Multiple users
* User accounts
* Permissions
* Household profiles
* Remote access

---

# 17. Multi-Device Playback

The MVP should prioritize browser-based playback.

The backend/API should be designed so that future clients can be created.

Potential future clients:

* Web
* Smart TV
* Android
* iOS
* Desktop
* Kodi/plugin ecosystem
* Other media clients

These clients are outside MVP scope.

---

# 18. Cloud / Paid Product Direction

The core open-source project should remain useful as a self-hosted product.

A future commercial service may provide convenience features such as:

* Hosted instances
* Remote access
* Cloud configuration synchronization
* Backups
* Multi-user management
* Managed updates
* Premium integrations
* Additional automation
* Advanced metadata services

These features should not influence the architecture of the MVP in ways that make the open-source version artificially limited.

---

# 19. Explicit MVP Non-Goals

Claude Code MUST NOT implement these unless a future specification explicitly requests them:

* Netflix integration
* Hulu integration
* YouTube integration
* Cloud media storage
* Torrent functionality
* Piracy-site integration
* DRM circumvention
* Media downloading services
* Social media features
* Public channel marketplace
* User-to-user media sharing
* Mobile applications
* Smart TV applications
* Complex recommendation systems
* AI-generated schedules
* Advanced authentication
* Payments
* Subscription billing
* Cloud infrastructure

These are future possibilities, not MVP requirements.

---

# 20. Suggested Initial Architecture

The exact architecture should be determined through a technical design/specification phase rather than assumed directly from this PRD.

However, the implementation should maintain clear separation between:

```text
Media Management
       │
       ▼
Scheduling
       │
       ▼
Channel State
       │
       ▼
Playback
       │
       ▼
API
       │
       ▼
Clients
```

The UI should consume the API rather than directly manipulating the underlying media/scheduling system.

---

# 21. MVP Success Criteria

The MVP is successful when a user can:

1. Install the application locally.
2. Configure a local media directory.
3. Scan and discover their videos.
4. Create multiple channels.
5. Add media to channels.
6. Create a schedule.
7. Start a channel.
8. Watch the currently scheduled media.
9. Switch between channels.
10. See what is currently playing.
11. See what plays next.
12. View the channel schedule through an EPG.
13. Restart the application without losing their configuration.
14. Run the complete system using Docker.

The fundamental test is:

> **Can a user take a folder full of their own videos and turn it into a functioning personalized TV network?**

If yes, the MVP has achieved its primary objective.

---

# 22. Development Philosophy

This PRD defines the product requirements, not the complete implementation.

Claude Code should NOT invent major product features simply because they appear technically interesting.

When implementation details are ambiguous:

1. Identify the ambiguity.
2. Check existing project documentation and specifications.
3. Prefer the simplest implementation consistent with this PRD.
4. Do not expand MVP scope without explicit approval.
5. If a decision materially affects architecture or future functionality, surface it before implementing.

The implementation should favor:

* Simple architecture
* Clear boundaries
* Testability
* Extensibility where it has a concrete purpose
* Minimal unnecessary dependencies
* Local-first operation
* Docker-first deployment
* API/UI separation

---

# 23. Future Product Direction

The long-term vision is to evolve the project from a personal media scheduler into a platform for creating personalized television networks.

Potential future capabilities include:

```text
Local Media
     │
     ├── Personal Channels
     ├── Automatic Programming
     ├── EPG
     ├── External Media Sources
     ├── Remote Access
     ├── Cloud Services
     └── Community/Shared Channels
```

However, the MVP should remain focused on one fundamental capability:

> **Turn locally owned/user-controlled media into scheduled, watchable television channels.**

---

# 24. Definition of the Product

In one sentence:

> **Personal TV is an open-source, self-hosted platform that turns a user's local media library into fully configurable virtual television channels with schedules and an electronic program guide.**

The first version should do this extremely well before attempting to become a broader media platform.
