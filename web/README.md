# Personal TV — frontend

This is the Personal TV (HomeStreamer) frontend: a React + TypeScript single-page
app that talks to the Go backend's REST API (`internal/api`) to manage media
sources, browse the library, configure channels and their schedules, and view
the electronic program guide (EPG).

## Development

```bash
npm install
npm run dev
```

`npm run dev` starts the Vite dev server and proxies `/api/*` requests to a
Go backend running on `:8080` (see `vite.config.ts`). Run the backend
alongside it from the repo root:

```bash
go run ./cmd/personaltv
```

## Testing

```bash
npm test
```

Tests are colocated with the code they test (e.g. `Foo.tsx` / `Foo.test.tsx`)
and use MSW to mock the API at the network boundary rather than mocking
`web/src/api/*` calls directly — the same black-box, real-dependencies spirit
as the Go backend's test conventions (see the repo root `CLAUDE.md`).

## Building

```bash
npm run build
```

This type-checks and produces a production build in `web/dist`. A real
`npm run build` must be run here **before** `go build`/`go run` at the repo
root for the embedded SPA (`web/embed.go`, via `go:embed`) to serve real
content — otherwise the Go binary only embeds the tracked placeholder files
in `web/dist`.
