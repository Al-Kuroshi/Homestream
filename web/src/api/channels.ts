import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";
import type { Channel } from "./types";

export function listChannels(): Promise<Channel[]> {
  return apiGet<Channel[]>("/channels");
}

export function getChannel(id: number): Promise<Channel> {
  return apiGet<Channel>(`/channels/${id}`);
}

export interface CreateChannelInput {
  name: string;
  description?: string;
  position?: number;
}

export function createChannel(input: CreateChannelInput): Promise<Channel> {
  return apiPost<Channel>("/channels", input);
}

export interface UpdateChannelInput {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  position: number;
}

export function updateChannel(input: UpdateChannelInput): Promise<Channel> {
  return apiPut<Channel>(`/channels/${input.id}`, {
    name: input.name,
    description: input.description,
    enabled: input.enabled,
    position: input.position,
  });
}

export function deleteChannel(id: number): Promise<void> {
  return apiDelete(`/channels/${id}`);
}

const channelsKey = ["channels"] as const;

export function useChannels() {
  return useQuery({ queryKey: channelsKey, queryFn: listChannels });
}

export function useChannel(id: number) {
  return useQuery({
    queryKey: [...channelsKey, id],
    queryFn: () => getChannel(id),
    enabled: id > 0,
  });
}

export function useCreateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsKey }),
  });
}

export function useUpdateChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsKey }),
  });
}

export function useDeleteChannel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteChannel,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: channelsKey }),
  });
}
