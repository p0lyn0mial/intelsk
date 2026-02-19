import { useState, useCallback } from 'react';
import type { SearchResult } from '../api/types';

interface VideoPlayerState {
  isOpen: boolean;
  sourceVideoUrl: string;
  seekOffsetSec: number;
  cameraName: string;
  timestamp: string;
}

export default function useVideoPlayer() {
  const [state, setState] = useState<VideoPlayerState>({
    isOpen: false,
    sourceVideoUrl: '',
    seekOffsetSec: 0,
    cameraName: '',
    timestamp: '',
  });

  const openVideo = useCallback((result: SearchResult) => {
    setState({
      isOpen: true,
      sourceVideoUrl: result.source_video_url || '',
      seekOffsetSec: result.seek_offset_sec,
      cameraName: result.camera_id,
      timestamp: result.timestamp,
    });
  }, []);

  const closeVideo = useCallback(() => {
    setState((prev) => ({ ...prev, isOpen: false }));
  }, []);

  return { state, openVideo, closeVideo };
}
