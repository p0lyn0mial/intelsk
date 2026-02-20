import type {
  CameraInfo,
  CameraDateStats,
  CreateCameraRequest,
  UpdateCameraRequest,
  VideoFile,
  ProcessRequest,
  ProcessResponse,
  ProcessHistoryEntry,
  TextSearchRequest,
  SearchResponse,
  ProgressEvent,
  SettingsMap,
  SettingsResponse,
} from './types';

const BASE = '/api';

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, options);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text}`);
  }
  return res.json();
}

export async function healthCheck(): Promise<{ status: string; ml_sidecar: string }> {
  return fetchJSON(`${BASE}/health`);
}

export async function getCameras(): Promise<CameraInfo[]> {
  return fetchJSON(`${BASE}/cameras`);
}

export async function getCamera(id: string): Promise<CameraInfo> {
  return fetchJSON(`${BASE}/cameras/${id}`);
}

export async function getCameraStats(id: string): Promise<CameraDateStats[]> {
  return fetchJSON(`${BASE}/cameras/${id}/stats`);
}

export async function createCamera(req: CreateCameraRequest): Promise<CameraInfo> {
  return fetchJSON(`${BASE}/cameras`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export async function updateCamera(id: string, req: UpdateCameraRequest): Promise<CameraInfo> {
  return fetchJSON(`${BASE}/cameras/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export async function deleteCamera(id: string, deleteData: boolean): Promise<void> {
  await fetchJSON(`${BASE}/cameras/${id}?delete_data=${deleteData}`, {
    method: 'DELETE',
  });
}

export async function getCameraVideos(id: string): Promise<VideoFile[]> {
  return fetchJSON(`${BASE}/cameras/${id}/videos`);
}

export async function deleteVideo(id: string, date: string, filename: string): Promise<void> {
  await fetchJSON(`${BASE}/cameras/${id}/videos?date=${encodeURIComponent(date)}&filename=${encodeURIComponent(filename)}`, {
    method: 'DELETE',
  });
}

export async function cleanCameraData(id: string, scope: 'videos' | 'all'): Promise<void> {
  await fetchJSON(`${BASE}/cameras/${id}/data?scope=${scope}`, {
    method: 'DELETE',
  });
}

export function uploadVideos(
  id: string,
  files: FileList | File[],
  onProgress?: (loaded: number, total: number) => void,
): Promise<{ status: string; paths: string[] }> {
  return new Promise((resolve, reject) => {
    const formData = new FormData();
    for (let i = 0; i < files.length; i++) {
      formData.append('files', files[i]);
    }
    const xhr = new XMLHttpRequest();
    xhr.open('POST', `${BASE}/cameras/${id}/upload`);
    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress(e.loaded, e.total);
      };
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(JSON.parse(xhr.responseText));
      } else {
        reject(new Error(`${xhr.status}: ${xhr.responseText}`));
      }
    };
    xhr.onerror = () => reject(new Error('Upload failed'));
    xhr.send(formData);
  });
}

export async function startProcess(req: ProcessRequest): Promise<ProcessResponse> {
  return fetchJSON(`${BASE}/process`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export function streamProcessStatus(
  jobId: string,
  onEvent: (event: ProgressEvent) => void,
  onDone: () => void,
): () => void {
  const eventSource = new EventSource(`${BASE}/process/status?job_id=${jobId}`);

  eventSource.onmessage = (e) => {
    const event: ProgressEvent = JSON.parse(e.data);
    onEvent(event);
    if (event.stage === 'complete') {
      eventSource.close();
      onDone();
    }
  };

  eventSource.onerror = () => {
    eventSource.close();
    onDone();
  };

  return () => eventSource.close();
}

export async function getProcessHistory(): Promise<ProcessHistoryEntry[]> {
  return fetchJSON(`${BASE}/process/history`);
}

export async function searchText(req: TextSearchRequest): Promise<SearchResponse> {
  return fetchJSON(`${BASE}/search/text`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export async function getSettings(): Promise<SettingsResponse> {
  return fetchJSON<SettingsResponse>(`${BASE}/settings`);
}

export async function updateSettings(settings: Partial<SettingsMap>): Promise<SettingsResponse> {
  return fetchJSON<SettingsResponse>(`${BASE}/settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ settings }),
  });
}
