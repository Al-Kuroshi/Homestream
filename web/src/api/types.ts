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

export interface Program {
  id: number;
  channel_id: number;
  media_item_id: number;
  start_time: string;
  created_at: string;
  updated_at: string;
}
