import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueries, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { getCameras, getCameraStats, getCameraSnapshotUrl } from '../api/client';
import type { CameraInfo, CameraDateStats } from '../api/types';
import {
  AddCameraModal,
  EditCameraModal,
  DeleteCameraDialog,
  UploadVideoModal,
} from '../components/CameraModals';
import { LiveStreamModal } from '../components/LiveStreamModal';

export default function CamerasPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: getCameras,
    refetchInterval: 10000,
  });

  const statsQueries = useQueries({
    queries: cameras.map((cam) => ({
      queryKey: ['cameraStats', cam.id],
      queryFn: () => getCameraStats(cam.id),
      enabled: cameras.length > 0,
    })),
  });

  const statsMap = cameras.reduce<Record<string, CameraDateStats[]>>((acc, cam, i) => {
    acc[cam.id] = statsQueries[i]?.data ?? [];
    return acc;
  }, {});

  const [snapshotTs] = useState(() => Date.now());
  const [showAdd, setShowAdd] = useState(false);
  const [editCamera, setEditCamera] = useState<CameraInfo | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CameraInfo | null>(null);
  const [uploadTarget, setUploadTarget] = useState<CameraInfo | null>(null);
  const [liveTarget, setLiveTarget] = useState<CameraInfo | null>(null);

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['cameras'] });
    queryClient.invalidateQueries({ queryKey: ['cameraStats'] });
  };

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-gray-900">
          {t('cameras.title')}
        </h1>
        <button
          onClick={() => setShowAdd(true)}
          className="px-4 py-2 text-sm text-white bg-blue-600 hover:bg-blue-700 rounded min-h-[44px]"
        >
          {t('cameras.add')}
        </button>
      </div>

      {cameras.length === 0 ? (
        <div className="text-center text-gray-500 py-12">
          {t('cameras.no_cameras')}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {cameras.map((cam) => (
            <div
              key={cam.id}
              className="bg-white rounded-lg shadow p-4 hover:shadow-md transition-shadow"
            >
              <Link to={`/cameras/${cam.id}`} className="block">
                <div className="mb-2 rounded overflow-hidden bg-gray-100 aspect-video">
                  <img
                    src={`${getCameraSnapshotUrl(cam.id)}?t=${snapshotTs}`}
                    alt={cam.name}
                    className="w-full h-full object-cover"
                    onError={(e) => {
                      (e.target as HTMLImageElement).style.display = 'none';
                    }}
                  />
                </div>
                <div className="flex items-center justify-between">
                  <h3 className="font-medium text-gray-900">{cam.name}</h3>
                  <div className="flex items-center gap-2">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full ${
                        cam.type === 'hikvision'
                          ? 'bg-purple-100 text-purple-700'
                          : 'bg-gray-100 text-gray-500'
                      }`}
                    >
                      {cam.type === 'hikvision' ? t('cameras.type_hikvision') : t('cameras.type_local')}
                    </span>
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full ${
                        cam.status === 'indexed'
                          ? 'bg-green-100 text-green-700'
                          : cam.status === 'online'
                            ? 'bg-blue-100 text-blue-700'
                            : 'bg-gray-100 text-gray-500'
                      }`}
                    >
                      {cam.status === 'indexed'
                        ? t('cameras.indexed')
                        : cam.status === 'online'
                          ? t('cameras.online')
                          : t('cameras.offline')}
                    </span>
                  </div>
                </div>
                <p className="text-sm text-gray-500 mt-1">{cam.id}</p>
                {statsMap[cam.id] && statsMap[cam.id].some((d) => d.video_count > 0) && (
                  <p className="text-xs text-gray-400 mt-1">
                    {t('cameras.stats_summary', {
                      dates: statsMap[cam.id].filter((d) => d.video_count > 0).length,
                      videos: statsMap[cam.id].reduce((s, d) => s + d.video_count, 0),
                      frames: statsMap[cam.id].reduce((s, d) => s + d.frame_count, 0),
                    })}
                  </p>
                )}
              </Link>
              <div className="flex items-center gap-2 mt-3 pt-3 border-t">
                {cam.created_at && (
                  <button
                    onClick={() => setEditCamera(cam)}
                    className="px-3 py-1.5 text-xs text-gray-600 hover:bg-gray-100 rounded min-h-[36px]"
                  >
                    {t('cameras.edit')}
                  </button>
                )}
                {cam.type === 'hikvision' ? (
                  <button
                    onClick={() => setLiveTarget(cam)}
                    className="px-3 py-1.5 text-xs text-green-600 hover:bg-green-50 rounded min-h-[36px] font-medium"
                  >
                    {t('cameras.live')}
                  </button>
                ) : (
                  <button
                    onClick={() => setUploadTarget(cam)}
                    className="px-3 py-1.5 text-xs text-blue-600 hover:bg-blue-50 rounded min-h-[36px]"
                  >
                    {t('cameras.upload')}
                  </button>
                )}
                <button
                  onClick={() => setDeleteTarget(cam)}
                  className="px-3 py-1.5 text-xs text-red-600 hover:bg-red-50 rounded ml-auto min-h-[36px]"
                >
                  {t('cameras.delete')}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      <AddCameraModal
        isOpen={showAdd}
        onClose={() => setShowAdd(false)}
        onCreated={refresh}
      />
      <EditCameraModal
        isOpen={editCamera !== null}
        camera={editCamera}
        onClose={() => setEditCamera(null)}
        onUpdated={refresh}
      />
      <DeleteCameraDialog
        isOpen={deleteTarget !== null}
        camera={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onDeleted={refresh}
      />
      <UploadVideoModal
        isOpen={uploadTarget !== null}
        camera={uploadTarget}
        onClose={() => setUploadTarget(null)}
        onUploaded={refresh}
      />
      <LiveStreamModal
        isOpen={liveTarget !== null}
        camera={liveTarget}
        onClose={() => setLiveTarget(null)}
      />
    </div>
  );
}
