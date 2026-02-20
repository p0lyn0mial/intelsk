import { useState, useEffect } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getCamera, getCameraStats, getCameraVideos, cleanCameraData, deleteVideo } from '../api/client';
import { LiveStreamModal } from '../components/LiveStreamModal';

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

type ConfirmModal =
  | { type: 'video'; date: string; filename: string }
  | { type: 'all' }
  | null;

export default function CameraDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const { data: camera } = useQuery({
    queryKey: ['camera', id],
    queryFn: () => getCamera(id!),
    enabled: !!id,
  });

  const { data: rawStats } = useQuery({
    queryKey: ['cameraStats', id],
    queryFn: () => getCameraStats(id!),
    enabled: !!id,
  });
  const stats = rawStats ?? [];

  const { data: rawVideos } = useQuery({
    queryKey: ['cameraVideos', id],
    queryFn: () => getCameraVideos(id!),
    enabled: !!id,
  });
  const videos = rawVideos ?? [];

  const [confirmModal, setConfirmModal] = useState<ConfirmModal>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [showLive, setShowLive] = useState(false);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (confirmModal) {
          setConfirmModal(null);
          setError('');
        } else {
          navigate('/cameras');
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [confirmModal, navigate]);

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['camera', id] });
    queryClient.invalidateQueries({ queryKey: ['cameraStats', id] });
    queryClient.invalidateQueries({ queryKey: ['cameraVideos', id] });
  };

  const handleConfirm = async () => {
    if (!confirmModal) return;
    setBusy(true);
    setError('');
    try {
      if (confirmModal.type === 'video') {
        await deleteVideo(id!, confirmModal.date, confirmModal.filename);
      } else {
        await cleanCameraData(id!, 'all');
      }
      setConfirmModal(null);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setBusy(false);
    }
  };

  // Build a map of date -> frame count from stats
  const frameCountByDate: Record<string, number> = {};
  for (const s of stats) {
    frameCountByDate[s.date] = s.frame_count;
  }

  // Group videos by date
  const videosByDate: Record<string, typeof videos> = {};
  for (const v of videos) {
    if (!videosByDate[v.date]) videosByDate[v.date] = [];
    videosByDate[v.date].push(v);
  }
  const dates = Object.keys(videosByDate).sort((a, b) => b.localeCompare(a));

  if (!camera) {
    return (
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <Link to="/cameras" className="text-sm text-blue-600 hover:underline">
          {t('detail.back')}
        </Link>
      </div>
    );
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-6">
      {/* Back link */}
      <Link to="/cameras" className="text-sm text-blue-600 hover:underline">
        {t('detail.back')}
      </Link>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-gray-900">{camera.name}</h1>
          <p className="text-sm text-gray-500">{camera.id}</p>
        </div>
        <div className="flex items-center gap-2">
          {camera.type === 'hikvision' && (
            <button
              onClick={() => setShowLive(true)}
              className="px-3 py-1.5 text-xs text-green-600 hover:bg-green-50 rounded border border-green-200 font-medium min-h-[36px]"
            >
              {t('cameras.live')}
            </button>
          )}
          <span
            className={`text-xs px-2 py-0.5 rounded-full ${
              camera.status === 'indexed'
                ? 'bg-green-100 text-green-700'
                : camera.status === 'online'
                  ? 'bg-blue-100 text-blue-700'
                  : 'bg-gray-100 text-gray-500'
            }`}
          >
            {camera.status === 'indexed'
              ? t('cameras.indexed')
              : camera.status === 'online'
                ? t('cameras.online')
                : t('cameras.offline')}
          </span>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="text-sm text-red-600 bg-red-50 rounded p-3">{error}</div>
      )}

      {/* Videos section */}
      {dates.length === 0 ? (
        <div className="text-center text-gray-500 py-12 bg-white rounded-lg shadow">
          {t('detail.no_videos')}
        </div>
      ) : (
        <div className="space-y-4">
          {dates.map((date) => {
            const frameCount = frameCountByDate[date] || 0;
            return (
              <div key={date} className="bg-white rounded-lg shadow">
                <div className="px-4 py-3 border-b flex items-center justify-between">
                  <h3 className="font-medium text-gray-900">{date}</h3>
                  {frameCount > 0 && (
                    <span className="text-xs text-gray-500">
                      {t('detail.frame_count', { count: frameCount })}
                    </span>
                  )}
                </div>
                <ul className="divide-y">
                  {videosByDate[date].map((v) => {
                    const key = `${v.date}/${v.filename}`;
                    return (
                      <li
                        key={key}
                        className="px-4 py-2 flex items-center justify-between text-sm"
                      >
                        <span className="text-gray-800 truncate">{v.filename}</span>
                        <div className="flex items-center gap-3 ml-4 shrink-0">
                          <span className="text-gray-400">{formatSize(v.size)}</span>
                          <button
                            onClick={() => setConfirmModal({ type: 'video', date: v.date, filename: v.filename })}
                            className="text-red-500 hover:text-red-700 min-h-[28px] px-1"
                            title={t('detail.delete_video')}
                          >
                            {'\u00d7'}
                          </button>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              </div>
            );
          })}
        </div>
      )}

      {/* Danger zone */}
      <div className="bg-white rounded-lg shadow border border-red-200">
        <div className="px-4 py-3 border-b border-red-200">
          <h3 className="font-medium text-red-600">{t('detail.danger_zone')}</h3>
        </div>
        <div className="px-4 py-4">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900">{t('detail.delete_all')}</p>
              <p className="text-xs text-gray-500">{t('detail.delete_all_desc')}</p>
            </div>
            <button
              onClick={() => setConfirmModal({ type: 'all' })}
              className="px-3 py-1.5 text-xs text-red-600 hover:bg-red-50 rounded border border-red-200 min-h-[36px]"
            >
              {t('detail.delete_all')}
            </button>
          </div>
        </div>
      </div>

      {/* Confirmation modal */}
      {confirmModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-white rounded-lg shadow-xl max-w-md w-full mx-4 p-6 space-y-4">
            <h3 className="text-lg font-semibold text-gray-900">
              {confirmModal.type === 'video'
                ? t('detail.delete_video')
                : t('detail.delete_all')}
            </h3>
            <p className="text-sm text-gray-600">
              {confirmModal.type === 'video'
                ? t('detail.delete_video_confirm', { filename: confirmModal.filename })
                : t('detail.delete_all_confirm')}
            </p>
            {error && (
              <div className="text-sm text-red-600 bg-red-50 rounded p-3">{error}</div>
            )}
            <div className="flex justify-end gap-2">
              <button
                onClick={() => { setConfirmModal(null); setError(''); }}
                disabled={busy}
                className="px-4 py-2 text-sm text-gray-600 hover:bg-gray-100 rounded min-h-[36px]"
              >
                {t('cameras.cancel')}
              </button>
              <button
                onClick={handleConfirm}
                disabled={busy}
                className="px-4 py-2 text-sm text-white bg-red-600 hover:bg-red-700 rounded disabled:opacity-50 min-h-[36px]"
              >
                {busy ? t('detail.cleaning') : t('detail.yes_delete')}
              </button>
            </div>
          </div>
        </div>
      )}

      <LiveStreamModal
        isOpen={showLive}
        camera={camera}
        onClose={() => setShowLive(false)}
      />
    </div>
  );
}
