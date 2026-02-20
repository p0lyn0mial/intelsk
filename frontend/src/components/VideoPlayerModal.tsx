import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface Props {
  isOpen: boolean;
  onClose: () => void;
  sourceVideoUrl: string;
  seekOffsetSec: number;
  cameraName: string;
  timestamp: string;
}

export default function VideoPlayerModal({
  isOpen,
  onClose,
  sourceVideoUrl,
  seekOffsetSec,
  cameraName,
  timestamp,
}: Props) {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState(false);

  const skip = (seconds: number) => {
    if (videoRef.current) {
      videoRef.current.currentTime = Math.max(0, videoRef.current.currentTime + seconds);
    }
  };

  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      if (e.key === 'ArrowLeft') { e.preventDefault(); skip(-10); }
      if (e.key === 'ArrowRight') { e.preventDefault(); skip(10); }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  useEffect(() => {
    if (isOpen) {
      setError(false);
    }
  }, [isOpen, sourceVideoUrl]);

  if (!isOpen) return null;

  const handleLoadedMetadata = () => {
    if (videoRef.current) {
      videoRef.current.currentTime = seekOffsetSec;
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4 sm:p-8"
      onClick={onClose}
    >
      <div
        className="bg-gray-900 rounded-lg w-full max-w-4xl max-h-[90vh] flex flex-col overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-700">
          <div className="text-sm text-gray-300">
            <span className="font-medium text-white">{cameraName}</span>
            <span className="mx-2">—</span>
            <span>{timestamp}</span>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-gray-700 rounded min-w-[44px] min-h-[44px] flex items-center justify-center"
            aria-label={t('video.close')}
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Video */}
        <div className="flex-1 min-h-0">
          {error ? (
            <div className="flex items-center justify-center h-64 text-gray-400">
              {t('video.error')}
            </div>
          ) : (
            <video
              ref={videoRef}
              src={sourceVideoUrl}
              className="w-full h-full object-contain"
              controls
              autoPlay
              muted
              onLoadedMetadata={handleLoadedMetadata}
              onError={() => setError(true)}
            />
          )}
        </div>

        {/* Skip controls + footer */}
        <div className="px-4 py-2 border-t border-gray-700 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <button
              onClick={() => skip(-10)}
              className="px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-700 rounded min-h-[36px]"
              title={t('video.skip_back')}
            >
              -10s
            </button>
            <button
              onClick={() => skip(10)}
              className="px-3 py-1.5 text-xs text-gray-300 hover:bg-gray-700 rounded min-h-[36px]"
              title={t('video.skip_forward')}
            >
              +10s
            </button>
          </div>
          <span className="text-xs text-gray-400">{t('video.seek_hint')}</span>
        </div>
      </div>
    </div>
  );
}
