import { fireEvent, render, screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { createTestQueryClient, wrapWithQueryClient } from "../test/queryClient";
import { server } from "../test/server";
import { ChannelSwitcher } from "./ChannelSwitcher";

const CHANNELS = [
  { id: 1, name: "Movies", description: "", enabled: true, position: 0, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 2, name: "Comedy", description: "", enabled: true, position: 1, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 3, name: "Off", description: "", enabled: false, position: 2, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

function renderSwitcher(currentChannelId: number, onSelect = vi.fn()) {
  server.use(http.get("/api/channels", () => HttpResponse.json(CHANNELS)));
  const client = createTestQueryClient();
  render(<ChannelSwitcher currentChannelId={currentChannelId} onSelect={onSelect} />, {
    wrapper: wrapWithQueryClient(client),
  });
  return onSelect;
}

async function waitForChannelsToLoad() {
  // Wait for channels to be fetched by opening the list and confirming it contains items
  fireEvent.click(screen.getByLabelText("Show channel list"));
  await screen.findByText("Movies");
  fireEvent.click(screen.getByLabelText("Show channel list"));
}

describe("ChannelSwitcher", () => {
  it("cycles to the next enabled channel", async () => {
    const onSelect = renderSwitcher(1);
    await waitForChannelsToLoad();
    fireEvent.click(screen.getByLabelText("Next channel"));
    expect(onSelect).toHaveBeenCalledWith(2);
  });

  it("wraps from the last enabled channel back to the first, skipping disabled ones", async () => {
    const onSelect = renderSwitcher(2);
    await waitForChannelsToLoad();
    fireEvent.click(screen.getByLabelText("Next channel"));
    expect(onSelect).toHaveBeenCalledWith(1);
  });

  it("cycles to the previous enabled channel", async () => {
    const onSelect = renderSwitcher(2);
    await waitForChannelsToLoad();
    fireEvent.click(screen.getByLabelText("Previous channel"));
    expect(onSelect).toHaveBeenCalledWith(1);
  });

  it("toggles a list of enabled channels and selects one directly", async () => {
    const onSelect = renderSwitcher(1);
    fireEvent.click(await screen.findByLabelText("Show channel list"));
    expect(await screen.findByText("Comedy")).toBeInTheDocument();
    expect(screen.queryByText("Off")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("Comedy"));
    expect(onSelect).toHaveBeenCalledWith(2);
  });
});
