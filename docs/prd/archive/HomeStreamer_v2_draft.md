# Personal TV — Product Requirements Document

**Status:** Draft
**Version:** 1.1
**Product Type:** Open-source, self-hosted personal television platform

---

# 1. Product Overview

Personal TV is a platform that allows users to transform their personal video library into a fully configurable television service.

Users can provide the platform with movies, television episodes, personal videos, recordings, and other compatible media. They can then create multiple virtual television channels and configure what each channel plays and when.

Instead of selecting individual videos from a traditional media library, users can turn on a channel and watch whatever is currently scheduled.

The long-term vision is to provide both:

1. A free, open-source, self-hosted version for users who want to run the platform themselves.
2. A hosted service for users who want the convenience of managed infrastructure and storage.

---

# 2. Product Vision

The goal is to allow anyone to create their own personalized television network.

A user should be able to take a collection such as:

* Movies
* TV shows
* Episodes
* Personal recordings
* Videos
* Other compatible media

and turn it into channels such as:

* Action Movies
* Comedy
* 90s Cartoons
* Anime
* Family TV
* Random Movies
* Classic TV

Each channel should operate independently and have its own programming schedule.

The desired experience is:

> Turn on a channel and watch what is currently playing, rather than deciding what to watch every time.

---

# 3. Core Concept

The platform consists of four primary concepts.

## 3.1 Media

Media represents playable content available to the platform.

Examples include:

* Movies
* TV episodes
* Short videos
* Personal recordings
* Other compatible video files

Media may eventually come from multiple sources.

The initial implementation should focus on user-controlled media.

---

## 3.2 Channels

A channel is a virtual television station.

Users can create any number of channels and configure each independently.

Examples:

```text
Channel 1 — Movies
Channel 2 — Comedy
Channel 3 — Anime
Channel 4 — Cartoons
Channel 5 — Family
```

Each channel has its own programming schedule.

---

## 3.3 Programs

A program represents a piece of content scheduled to play on a channel.

A program may represent:

* A specific movie
* A specific TV episode
* A playlist
* A collection of media
* A future automated scheduling rule

---

## 3.4 Schedule

A schedule determines what plays on a channel and when.

For example:

```text
18:00  Movie
20:00  TV Episode
20:30  TV Episode
21:00  Movie
```

The platform should automatically calculate program durations and determine what is currently playing.

---

# 4. Target Users

## Primary Users

Individuals who:

* Own a personal media collection
* Have local storage or a NAS
* Want a more passive way of watching their media
* Enjoy customizing their entertainment experience
* Are comfortable using self-hosted software

## Secondary Users

Households that want to create customized television channels from their own media collections.

## Future Users

Customers who want the same experience without managing their own hardware or storage.

---

# 5. Product Principles

## 5.1 Television First

The product should feel like a television service, not simply a media browser with a scheduling feature.

The primary experience should be:

```text
Choose Channel
      ↓
Watch Current Program
      ↓
See What's Next
```

---

## 5.2 User Control

Users should retain control over their media and how it is programmed.

The platform should not require users to surrender control of their media simply to use the core product.

---

## 5.3 Self-Hosted First

The open-source version should be fully usable on a user's own hardware.

Docker should be supported as a primary deployment method.

---

## 5.4 Extensible Media Sources

The scheduling system should not be tightly coupled to local files.

The platform should be designed around the concept of media sources so that additional sources can be introduced later.

Potential sources include:

* Local storage
* NAS
* Network storage
* User-controlled cloud storage
* Plex
* Jellyfin
* YouTube
* Other legally supported services

---

# 6. MVP Scope

The MVP should focus on creating a complete personal television experience from user-controlled media.

## 6.1 Media Library

Users should be able to:

* Configure one or more media directories
* Scan directories
* Discover compatible video files
* View discovered media
* View basic metadata
* Search media
* Filter media
* Rescan media sources
* Remove media sources

The platform should reference existing media rather than unnecessarily duplicating it.

---

# 7. Channel Management

Users should be able to:

* Create channels
* Rename channels
* Delete channels
* Reorder channels
* Enable or disable channels
* Configure channel descriptions
* Configure channel artwork where supported

Each channel should maintain an independent schedule.

---

# 8. Scheduling

The MVP should support straightforward sequential scheduling.

Example:

```text
18:00 → Movie A
20:05 → Episode 1
20:35 → Episode 2
21:05 → Movie B
```

The platform should automatically calculate program end times using media duration.

Users should be able to:

* Add media to a schedule
* Remove scheduled media
* Reorder programs
* Change scheduling times
* View upcoming programs
* Modify schedules

---

# 9. Future Scheduling Capabilities

The architecture should allow more sophisticated scheduling to be added later.

Potential capabilities include:

### Recurring Programs

```text
Every day at 20:00
→ Movie
```

### Random Selection

```text
21:00
→ Random movie from "Action"
```

### Collections

```text
22:00
→ Random episode from "Comedy Shows"
```

### Rotation Rules

```text
Do not repeat the same movie
within 30 days.
```

### Time-Based Programming

```text
06:00–09:00 → Kids
09:00–17:00 → General
17:00–20:00 → Family
20:00–00:00 → Movies
```

### Special Events

```text
Friday 21:00
→ Movie Night
```

These capabilities are not required for the initial MVP.

---

# 10. Playback

Users should be able to:

* Select a channel
* Watch the currently scheduled program
* Switch between channels
* See the current program title
* See program progress
* See the next program
* Access standard playback controls

The system should determine the appropriate playback position based on the current channel schedule.

For example, if a program started at 20:00 and the user joins at 20:25, playback should begin at approximately the corresponding position in the program.

---

# 11. Electronic Program Guide

The platform should provide an EPG-style interface.

The guide should display:

* Channels
* Current programs
* Upcoming programs
* Program start times
* Program end times

Example:

```text
             18:00       19:00       20:00       21:00

Movies       Movie A                 Movie B
Comedy                   Show A      Show B
Anime        Episode 1   Episode 2   Movie
```

The EPG should make it possible to understand what is currently playing and what will play later.

---

# 12. User Interface

The application should have several primary areas.

## TV

The primary viewing interface.

Should provide:

* Video player
* Current channel
* Current program
* Program progress
* Next program
* Channel selection

---

## Guide

A television-style EPG.

Users should be able to browse current and upcoming programming.

---

## Media Library

Users should be able to:

* Browse media
* Search media
* Filter media
* View metadata
* Select media for programming

---

## Channels

Users should be able to:

* Create channels
* Edit channels
* Delete channels
* Configure schedules

---

## Settings

Settings should include:

* Media sources
* Playback configuration
* Application configuration
* System configuration

---

# 13. Storage Models

The platform should support multiple deployment and storage models over its lifetime.

## 13.1 Self-Hosted Storage

The user's media remains on their own hardware.

Example:

```text
User's Computer / NAS
│
├── Movies
├── TV
└── Videos
        │
        ▼
   Personal TV
```

This is the primary storage model for the open-source MVP.

---

## 13.2 Bring Your Own Cloud Storage

A future hosted version may allow users to connect their own cloud storage.

Potential providers include:

* Amazon S3
* Cloudflare R2
* Backblaze B2
* Google Cloud Storage
* Other compatible storage providers

The platform would manage the television experience while the user retains control over the underlying storage.

---

## 13.3 Managed Storage

A future commercial service may offer storage directly.

For example:

```text
Personal TV Cloud

500 GB
2 TB
5 TB
10 TB
```

Storage capacity and pricing would be determined based on operational costs.

This model is outside the MVP.

---

# 14. Hosted Service

The long-term hosted product should remove the need for users to manage their own infrastructure.

A hosted customer could potentially receive:

* Managed application hosting
* Media storage
* Automatic backups
* Remote access
* Account management
* Automatic updates
* EPG
* Channel scheduling
* Multi-device playback

The hosted service should be treated as a separate commercial layer around the core product.

The open-source version should remain useful independently.

---

# 15. External Media Integrations

External services may eventually become optional integrations.

Potential integrations include:

* YouTube
* Plex
* Jellyfin
* User-controlled cloud storage
* Other compatible media services

These integrations should be implemented through a modular media-source system.

The core scheduler should not contain service-specific logic.

---

# 16. Streaming Service Integrations

Commercial streaming platforms such as Netflix, Hulu, and similar services require special consideration.

The platform should not:

* Circumvent DRM
* Download protected streams
* Rehost protected content
* Bypass authentication
* Circumvent access restrictions

Where supported by the relevant service, future integrations may provide capabilities such as:

* Metadata
* Search
* Deep links
* Playback handoff
* Service-specific account integration

The exact functionality should depend on the capabilities and terms of each service.

These integrations are outside the MVP.

---

# 17. Copyright and User Content

The open-source application is intended to provide infrastructure for users' own media libraries.

The MVP should not provide features specifically intended to facilitate copyright infringement.

The platform should not include:

* Piracy search engines
* Torrent search/download functionality
* Piracy-site scraping
* DRM circumvention
* DRM removal
* Centralized libraries of copyrighted content
* Unauthorized redistribution of media

Users remain responsible for ensuring that they have appropriate rights to the media they use.

If a future hosted service stores user media, the company must establish appropriate policies and legal procedures for user-uploaded content before offering that service.

---

# 18. API

The platform should expose an API for its core functionality.

The API should provide operations for:

* Media sources
* Media items
* Channels
* Programs
* Schedules
* Current playback state
* EPG data

The API should be designed so that future clients can consume it independently of the web interface.

Potential future clients include:

* Web
* Mobile
* Desktop
* Smart TV
* Other media-player clients

---

# 19. Docker and Deployment

The open-source application should support Docker-based deployment.

A user should be able to deploy the application without requiring a complex infrastructure setup.

The initial deployment should prioritize:

* Docker
* Docker Compose
* Local storage
* NAS storage
* Simple configuration

---

# 20. Reliability

The platform should continue operating correctly across normal failures.

It should handle:

* Application restarts
* Missing media files
* Deleted media
* Unreadable media
* Invalid media
* Temporary playback failures

A single unavailable media item should not cause an entire channel to stop functioning.

---

# 21. Performance

The platform should be capable of managing a reasonably large personal media collection.

Media scanning should not require loading the entire collection into memory.

Where possible, scanning should be incremental and avoid unnecessary reprocessing of unchanged files.

The scheduler should be lightweight and capable of managing multiple channels simultaneously.

---

# 22. Authentication and Users

Authentication is not required for the initial local-only MVP if it adds unnecessary complexity.

The architecture should allow authentication to be introduced later.

Future functionality may include:

* Multiple users
* User accounts
* Profiles
* Permissions
* Household accounts
* Parental controls
* Shared channels

---

# 23. Open-Source Business Model

The core self-hosted application should remain open source.

Potential future revenue sources include:

* Hosted Personal TV
* Managed storage
* Premium integrations
* Remote access
* Cloud backups
* Managed updates
* Enterprise functionality
* Premium support

The free version should remain a functional product rather than being intentionally crippled to force users into the hosted service.

---

# 24. MVP Non-Goals

The following should not be implemented in the MVP:

* Netflix integration
* Hulu integration
* YouTube integration
* Managed cloud storage
* Cloud backups
* Payments
* Subscriptions
* User marketplace
* Public channel sharing
* Social features
* Mobile applications
* Smart TV applications
* Advanced recommendation systems
* AI-generated programming
* DRM-related functionality
* Torrent functionality
* Piracy functionality
* Advanced multi-user authentication

These may be considered in future product versions.

---

# 25. MVP User Journey

A successful first-time experience should look approximately like:

```text
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

The user should be able to reach this point without complicated configuration.

---

# 26. MVP Success Criteria

The MVP is successful when a user can:

1. Install the application.
2. Configure local media storage.
3. Scan and discover videos.
4. Browse their media library.
5. Create multiple channels.
6. Add media to channels.
7. Create channel schedules.
8. Automatically determine current and upcoming programs.
9. Watch a channel.
10. Switch between channels.
11. View an electronic program guide.
12. Restart the application without losing configuration.
13. Run the application using Docker.

The central success criterion is:

> **A user can take their existing video collection and turn it into a functioning personalized television network.**

---

# 27. Long-Term Vision

The long-term product should evolve from a self-hosted media scheduler into a complete personalized television platform.

Potential evolution:

```text
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

The core idea should remain simple:

> **Give users the ability to create and operate their own television channels from media they control.**

The product should prioritize making that experience excellent before expanding into a broader media ecosystem.
