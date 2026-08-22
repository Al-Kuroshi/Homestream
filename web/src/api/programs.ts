import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";
import type { Program } from "./types";

export function listPrograms(channelId: number): Promise<Program[]> {
  return apiGet<Program[]>(`/channels/${channelId}/programs`);
}

export interface AddProgramInput {
  channelId: number;
  media_item_id: number;
  start_time: string;
}

export function addProgram(input: AddProgramInput): Promise<Program> {
  return apiPost<Program>(`/channels/${input.channelId}/programs`, {
    media_item_id: input.media_item_id,
    start_time: input.start_time,
  });
}

export interface UpdateProgramInput {
  id: number;
  channelId: number;
  media_item_id: number;
  start_time: string;
}

export function updateProgram(input: UpdateProgramInput): Promise<Program> {
  return apiPut<Program>(`/programs/${input.id}`, {
    media_item_id: input.media_item_id,
    start_time: input.start_time,
  });
}

export interface DeleteProgramInput {
  id: number;
  channelId: number;
}

export function deleteProgram(input: DeleteProgramInput): Promise<void> {
  return apiDelete(`/programs/${input.id}`);
}

function programsKey(channelId: number) {
  return ["channels", channelId, "programs"] as const;
}

export function useProgramsForChannel(channelId: number) {
  return useQuery({
    queryKey: programsKey(channelId),
    queryFn: () => listPrograms(channelId),
    enabled: channelId > 0,
  });
}

export function useAddProgram(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: addProgram,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: programsKey(channelId) }),
  });
}

export function useUpdateProgram(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateProgram,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: programsKey(channelId) }),
  });
}

export function useDeleteProgram(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteProgram,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: programsKey(channelId) }),
  });
}
