import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiDelete, apiGet, apiPost, apiPut } from "./http";
import type { ResolvedSlot, Slot } from "./types";

export function listSlots(channelId: number): Promise<Slot[]> {
  return apiGet<Slot[]>(`/channels/${channelId}/slots`);
}

export function listResolvedSlots(channelId: number, from: string, to: string): Promise<ResolvedSlot[]> {
  const params = new URLSearchParams({ from, to });
  return apiGet<ResolvedSlot[]>(`/channels/${channelId}/slots/resolved?${params}`);
}

export interface SlotInput {
  kind: "media" | "gap";
  media_item_id?: number;
  gap_duration_sec?: number;
  gap_label?: string;
  recurring: boolean;
  day_of_week?: number;
  position?: number;
  start_time?: string;
}

export interface AddSlotInput extends SlotInput {
  channelId: number;
}

export function addSlot(input: AddSlotInput): Promise<Slot> {
  const { channelId, ...body } = input;
  return apiPost<Slot>(`/channels/${channelId}/slots`, body);
}

export interface UpdateSlotInput extends SlotInput {
  id: number;
  channelId: number;
}

export function updateSlot(input: UpdateSlotInput): Promise<Slot> {
  const { id, channelId, ...body } = input;
  void channelId;
  return apiPut<Slot>(`/slots/${id}`, body);
}

export interface DeleteSlotInput {
  id: number;
  channelId: number;
}

export function deleteSlot(input: DeleteSlotInput): Promise<void> {
  return apiDelete(`/slots/${input.id}`);
}

function slotsKey(channelId: number) {
  return ["channels", channelId, "slots"] as const;
}

function resolvedSlotsBaseKey(channelId: number) {
  return ["channels", channelId, "slots", "resolved"] as const;
}

export function useSlotsForChannel(channelId: number) {
  return useQuery({ queryKey: slotsKey(channelId), queryFn: () => listSlots(channelId), enabled: channelId > 0 });
}

export function useResolvedSlots(channelId: number, from: string, to: string) {
  return useQuery({
    queryKey: [...resolvedSlotsBaseKey(channelId), from, to] as const,
    queryFn: () => listResolvedSlots(channelId, from, to),
    enabled: channelId > 0,
  });
}

function invalidateChannelSlots(queryClient: ReturnType<typeof useQueryClient>, channelId: number) {
  queryClient.invalidateQueries({ queryKey: slotsKey(channelId) });
  queryClient.invalidateQueries({ queryKey: resolvedSlotsBaseKey(channelId) });
}

export function useAddSlot(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: addSlot,
    onSuccess: () => invalidateChannelSlots(queryClient, channelId),
  });
}

export function useUpdateSlot(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: updateSlot,
    onSuccess: () => invalidateChannelSlots(queryClient, channelId),
  });
}

export function useDeleteSlot(channelId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: deleteSlot,
    onSuccess: () => invalidateChannelSlots(queryClient, channelId),
  });
}
