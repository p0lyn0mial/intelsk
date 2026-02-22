export interface CameraInfo {
  id: string;
  name: string;
  type: string;
  config: Record<string, unknown>;
  status: string;
  created_at?: string;
  updated_at?: string;
}

export interface CreateCameraRequest {
  id: string;
  name: string;
  type: string;
  config?: Record<string, unknown>;
}

export interface UpdateCameraRequest {
  name?: string;
  config?: Record<string, unknown>;
}

export interface ProcessRequest {
  camera_ids: string[];
  start_date: string;
  end_date: string;
  start_time?: string;
  end_time?: string;
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

export interface VideoFile {
  date: string;
  filename: string;
  size: number;
}

export interface CameraDateStats {
  date: string;
  video_count: number;
  frame_count: number;
}

export interface ProcessHistoryEntry {
  camera_id: string;
  date: string;
  videos?: string[];
  indexed_at: string;
}

export interface UploadJobEvent {
  stage: string;
  file?: string;
  current?: number;
  total?: number;
  frames_done?: number;
  frames_total?: number;
}

export interface ModelInfo {
  preset: string;
  model: string;
  embedding_dim: number;
  presets?: Record<string, string>;
  status?: string;
}

export type SettingsMap = Record<string, number | boolean | string>;

export interface SettingsResponse {
  settings: SettingsMap;
  defaults: SettingsMap;
}
