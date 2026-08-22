import { useQuery } from "@tanstack/react-query";
import { apiGet } from "./http";
import type { MediaItem } from "./types";

export function listMedia(): Promise<MediaItem[]> {
  return apiGet<MediaItem[]>("/media");
}

export function useMediaItems() {
  return useQuery({ queryKey: ["media"], queryFn: listMedia });
}
