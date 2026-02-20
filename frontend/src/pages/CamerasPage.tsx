import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { getCameras } from '../api/client';
import type { CameraInfo } from '../api/types';
import {
  AddCameraModal,
  EditCameraModal,
  DeleteCameraDialog,
  DownloadVideoModal,
} from '../components/CameraModals';

export default function CamerasPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: getCameras,
    refetchInterval: 10000,
  });

  const [showAdd, setShowAdd] = useState(false);
  const [editCamera, setEditCamera] = useState<CameraInfo | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<CameraInfo | null>(null);
  const [downloadTarget, setDownloadTarget] = useState<CameraInfo | null>(null);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['cameras'] });

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
              <Link to={`/?camera=${cam.id}`} className="block">
                <div className="flex items-center justify-between">
                  <h3 className="font-medium text-gray-900">{cam.name}</h3>
                  <div className="flex items-center gap-2">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full ${
                        cam.type === 'test'
                          ? 'bg-purple-100 text-purple-700'
                          : 'bg-gray-100 text-gray-500'
                      }`}
                    >
                      {t(`cameras.type_${cam.type}`)}
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
                {cam.type === 'test' && (
                  <button
                    onClick={() => setDownloadTarget(cam)}
                    className="px-3 py-1.5 text-xs text-blue-600 hover:bg-blue-50 rounded min-h-[36px]"
                  >
                    {t('cameras.download')}
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
      <DownloadVideoModal
        isOpen={downloadTarget !== null}
        camera={downloadTarget}
        onClose={() => setDownloadTarget(null)}
        onDownloaded={refresh}
      />
    </div>
  );
}
