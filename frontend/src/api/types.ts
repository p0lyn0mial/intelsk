export interface CameraInfo {
  id: string;
  name: string;
  status: string;
}

export interface ProcessRequest {
  camera_ids: string[];
  start_date: string;
  end_date: string;
}

export interface ProcessResponse {
  job_id: string;
  status: string;
}

export interface ProgressEvent {
  stage: string;
  camera_id?: string;
  frames_done?: number;
  frames_total?: number;
  message?: string;
}

export interface TextSearchRequest {
  query: string;
  camera_ids?: string[];
  start_time?: string;
  end_time?: string;
  limit?: number;
}

export interface SearchResult {
  frame_id: string;
  frame_url: string;
  camera_id: string;
  timestamp: string;
  score: number;
  source_video_url?: string;
  seek_offset_sec: number;
}

export interface SearchResponse {
  results: SearchResult[];
  query: string;
  total: number;
}

export interface ProcessHistoryEntry {
  camera_id: string;
  date: string;
  videos?: string[];
  indexed_at: string;
}

export type SettingsMap = Record<string, number | boolean>;

export interface SettingsResponse {
  settings: SettingsMap;
  defaults: SettingsMap;
}
