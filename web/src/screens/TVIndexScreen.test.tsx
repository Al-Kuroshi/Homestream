import { render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { TVIndexScreen } from "./TVIndexScreen";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Comedy", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

// Reads the matched route param from React Router's in-memory location
// (not window.location, which MemoryRouter never touches) so the test can
// assert which channel TVIndexScreen actually redirected to.
function LandedOnChannel() {
  const { channelId } = useParams<{ channelId: string }>();
  return <p>Landed on channel {channelId}</p>;
}

function renderIndex(channels: typeof CHANNELS) {
  server.use(http.get("/api/channels", () => HttpResponse.json(channels)));
  const client = createTestQueryClient();
  render(
    <MemoryRouter initialEntries={["/tv"]}>
      <Routes>
        <Route path="/tv" element={<TVIndexScreen />} />
        <Route path="/tv/:channelId" element={<LandedOnChannel />} />
      </Routes>
    </MemoryRouter>,
    { wrapper: wrapWithQueryClient(client) }
  );
}

describe("TVIndexScreen", () => {
  beforeEach(() => localStorage.clear());

  it("redirects to the first enabled channel when there's no last-watched channel", async () => {
    renderIndex(CHANNELS);
    expect(await screen.findByText("Landed on channel 1")).toBeInTheDocument();
  });

  it("redirects to the last-watched channel when it's still enabled", async () => {
    localStorage.setItem("personaltv.tv.lastChannelId", "2");
    renderIndex(CHANNELS);
    expect(await screen.findByText("Landed on channel 2")).toBeInTheDocument();
  });

  it("falls back to the first enabled channel when the last-watched one is gone or disabled", async () => {
    localStorage.setItem("personaltv.tv.lastChannelId", "999");
    renderIndex(CHANNELS);
    expect(await screen.findByText("Landed on channel 1")).toBeInTheDocument();
  });

  it("shows an empty state instead of redirecting when there are no enabled channels", async () => {
    renderIndex([]);
    expect(await screen.findByText(/No channels yet/)).toBeInTheDocument();
  });
});
