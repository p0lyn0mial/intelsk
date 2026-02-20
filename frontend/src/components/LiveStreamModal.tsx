import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Hls from 'hls.js';
import type { CameraInfo } from '../api/types';
import { startStream, stopStream, getStreamPlaylistUrl } from '../api/client';

interface LiveStreamModalProps {
  isOpen: boolean;
  camera: CameraInfo | null;
  onClose: () => void;
}

export function LiveStreamModal({ isOpen, camera, onClose }: LiveStreamModalProps) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!isOpen || !camera) return;

    let cancelled = false;
    setLoading(true);
    setError('');

    const init = async () => {
      try {
        await startStream(camera.id);
        if (cancelled) return;

        const url = getStreamPlaylistUrl(camera.id);

        // Wait for the HLS playlist to become available
        const waitForPlaylist = async () => {
          for (let i = 0; i < 20; i++) {
            if (cancelled) return false;
            try {
              const resp = await fetch(url);
              if (resp.ok) return true;
            } catch {
              // not ready yet
            }
            await new Promise((r) => setTimeout(r, 500));
          }
          return false;
        };

        const ready = await waitForPlaylist();
        if (cancelled) return;
        if (!ready) {
          setError('Stream not available');
          setLoading(false);
          return;
        }

        if (Hls.isSupported() && videoRef.current) {
          const hls = new Hls({
            enableWorker: true,
            lowLatencyMode: true,
          });
          hls.loadSource(url);
          hls.attachMedia(videoRef.current);
          hls.on(Hls.Events.MANIFEST_PARSED, () => {
            if (!cancelled && videoRef.current) {
              videoRef.current.play().catch(() => {});
              setLoading(false);
            }
          });
          hls.on(Hls.Events.ERROR, (_event, data) => {
            if (data.fatal && !cancelled) {
              setError('Stream error');
              setLoading(false);
            }
          });
          hlsRef.current = hls;
        } else if (videoRef.current?.canPlayType('application/vnd.apple.mpegurl')) {
          // Native HLS support (Safari)
          videoRef.current.src = url;
          videoRef.current.addEventListener('loadedmetadata', () => {
            if (!cancelled && videoRef.current) {
              videoRef.current.play().catch(() => {});
              setLoading(false);
            }
          });
        } else {
          setError('HLS not supported');
          setLoading(false);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to start stream');
          setLoading(false);
        }
      }
    };

    init();

    return () => {
      cancelled = true;
      if (hlsRef.current) {
        hlsRef.current.destroy();
        hlsRef.current = null;
      }
      if (camera) {
        stopStream(camera.id).catch(() => {});
      }
    };
  }, [isOpen, camera]);

  useEffect(() => {
    if (!isOpen) return;
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [isOpen, onClose]);

  if (!isOpen || !camera) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4"
      onClick={onClose}
    >
      <div
        className="bg-gray-900 rounded-lg shadow-xl w-full max-w-4xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="px-4 py-3 flex items-center justify-between border-b border-gray-700">
          <div className="flex items-center gap-3">
            <h2 className="text-white font-medium">{camera.name}</h2>
            <span className="px-2 py-0.5 text-xs font-bold text-white bg-red-600 rounded">
              {t('cameras.live_indicator')}
            </span>
          </div>
          <button
            onClick={onClose}
            className="text-gray-400 hover:text-white text-xl leading-none px-2"
          >
            {'\u00d7'}
          </button>
        </div>
        <div className="relative aspect-video bg-black">
          {loading && (
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="text-center">
                <svg
                  className="animate-spin h-8 w-8 text-blue-500 mx-auto mb-2"
                  viewBox="0 0 24 24"
                  fill="none"
                >
                  <circle
                    className="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    strokeWidth="4"
                  />
                  <path
                    className="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                  />
                </svg>
                <p className="text-gray-400 text-sm">{t('cameras.live_loading')}</p>
              </div>
            </div>
          )}
          {error && (
            <div className="absolute inset-0 flex items-center justify-center">
              <p className="text-red-400 text-sm">{error}</p>
            </div>
          )}
          <video
            ref={videoRef}
            className="w-full h-full"
            autoPlay
            muted
            playsInline
          />
        </div>
      </div>
    </div>
  );
}
