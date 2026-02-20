import type {
  CameraInfo,
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

export async function getSettings(): Promise<SettingsMap> {
  const res = await fetchJSON<SettingsResponse>(`${BASE}/settings`);
  return res.settings;
}

export async function updateSettings(settings: Partial<SettingsMap>): Promise<SettingsMap> {
  const res = await fetchJSON<SettingsResponse>(`${BASE}/settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ settings }),
  });
  return res.settings;
}
