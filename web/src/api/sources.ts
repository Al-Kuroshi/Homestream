import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost } from "./http";
import type { MediaSource } from "./types";

export function listSources(): Promise<MediaSource[]> {
  return apiGet<MediaSource[]>("/sources");
}

export interface CreateSourceInput {
  name: string;
  path: string;
}

export function createSource(input: CreateSourceInput): Promise<MediaSource> {
  return apiPost<MediaSource>("/sources", input);
}

export function deleteSource(id: number): Promise<void> {
  return apiDelete(`/sources/${id}`);
}

export function scanSource(id: number): Promise<void> {
  return apiPost<void>(`/sources/${id}/scan`);
}

const sourcesKey = ["sources"] as const;
const mediaKey = ["media"] as const;

export function useSources() {
  return useQuery({ queryKey: sourcesKey, queryFn: listSources });
}

export function useCreateSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createSource,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: sourcesKey }),
  });
}

export function useDeleteSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteSource,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: sourcesKey }),
  });
}

export function useScanSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: scanSource,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sourcesKey });
      queryClient.invalidateQueries({ queryKey: mediaKey });
    },
  });
}
