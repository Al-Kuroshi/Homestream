export interface MediaSource {
  id: number;
  name: string;
  path: string;
  created_at: string;
}

export interface MediaItem {
  id: number;
  source_id: number;
  rel_path: string;
  title: string;
  duration_sec: number;
  video_codec: string;
  audio_codec: string;
  container: string;
  size_bytes: number;
  mod_time: string;
  invalid: boolean;
  created_at: string;
  updated_at: string;
}

export interface Channel {
  id: number;
  name: string;
  description: string;
  enabled: boolean;
  position: number;
  created_at: string;
  updated_at: string;
}

export interface Slot {
  id: number;
  channel_id: number;
  kind: "media" | "gap";
  media_item_id: number | null;
  gap_duration_sec: number | null;
  gap_label: string;
  recurring: boolean;
  day_of_week: number | null;
  position: number | null;
  start_time: string | null;
  created_at: string;
  updated_at: string;
}

export interface ResolvedSlot {
  program_id: number;
  media_item_id: number;
  start_time: string;
  end_time: string;
}
