import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getCamera, getCameraStats, getCameraVideos, cleanCameraData, deleteVideo } from '../api/client';

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

export default function CameraDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const { data: camera } = useQuery({
    queryKey: ['camera', id],
    queryFn: () => getCamera(id!),
    enabled: !!id,
  });

  const { data: stats = [] } = useQuery({
    queryKey: ['cameraStats', id],
    queryFn: () => getCameraStats(id!),
    enabled: !!id,
  });

  const { data: videos = [] } = useQuery({
    queryKey: ['cameraVideos', id],
    queryFn: () => getCameraVideos(id!),
    enabled: !!id,
  });

  const [confirmDelete, setConfirmDelete] = useState<'videos' | 'all' | null>(null);
  const [cleaning, setCleaning] = useState(false);
  const [error, setError] = useState('');
  const [deletingVideo, setDeletingVideo] = useState<string | null>(null);

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['camera', id] });
    queryClient.invalidateQueries({ queryKey: ['cameraStats', id] });
    queryClient.invalidateQueries({ queryKey: ['cameraVideos', id] });
  };

  const handleClean = async (scope: 'videos' | 'all') => {
    setCleaning(true);
    setError('');
    try {
      await cleanCameraData(id!, scope);
      setConfirmDelete(null);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setCleaning(false);
    }
  };

  const handleDeleteVideo = async (date: string, filename: string) => {
    const key = `${date}/${filename}`;
    setDeletingVideo(key);
    setError('');
    try {
      await deleteVideo(id!, date, filename);
      refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setDeletingVideo(null);
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
                            onClick={() => handleDeleteVideo(v.date, v.filename)}
                            disabled={deletingVideo === key}
                            className="text-red-500 hover:text-red-700 disabled:opacity-50 min-h-[28px] px-1"
                            title={t('detail.delete_video')}
                          >
                            {deletingVideo === key ? '...' : '\u00d7'}
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
        <div className="px-4 py-4 space-y-3">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900">{t('detail.delete_videos')}</p>
              <p className="text-xs text-gray-500">{t('detail.delete_videos_desc')}</p>
            </div>
            {confirmDelete === 'videos' ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-red-600">{t('detail.confirm')}</span>
                <button
                  onClick={() => handleClean('videos')}
                  disabled={cleaning}
                  className="px-3 py-1.5 text-xs text-white bg-red-600 hover:bg-red-700 rounded disabled:opacity-50 min-h-[36px]"
                >
                  {cleaning ? t('detail.cleaning') : t('detail.yes_delete')}
                </button>
                <button
                  onClick={() => setConfirmDelete(null)}
                  className="px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-100 rounded min-h-[36px]"
                >
                  {t('cameras.cancel')}
                </button>
              </div>
            ) : (
              <button
                onClick={() => setConfirmDelete('videos')}
                className="px-3 py-1.5 text-xs text-red-600 hover:bg-red-50 rounded border border-red-200 min-h-[36px]"
              >
                {t('detail.delete_videos')}
              </button>
            )}
          </div>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm font-medium text-gray-900">{t('detail.delete_all')}</p>
              <p className="text-xs text-gray-500">{t('detail.delete_all_desc')}</p>
            </div>
            {confirmDelete === 'all' ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-red-600">{t('detail.confirm')}</span>
                <button
                  onClick={() => handleClean('all')}
                  disabled={cleaning}
                  className="px-3 py-1.5 text-xs text-white bg-red-600 hover:bg-red-700 rounded disabled:opacity-50 min-h-[36px]"
                >
                  {cleaning ? t('detail.cleaning') : t('detail.yes_delete')}
                </button>
                <button
                  onClick={() => setConfirmDelete(null)}
                  className="px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-100 rounded min-h-[36px]"
                >
                  {t('cameras.cancel')}
                </button>
              </div>
            ) : (
              <button
                onClick={() => setConfirmDelete('all')}
                className="px-3 py-1.5 text-xs text-red-600 hover:bg-red-50 rounded border border-red-200 min-h-[36px]"
              >
                {t('detail.delete_all')}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
