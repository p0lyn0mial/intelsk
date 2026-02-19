import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { getCameras } from '../api/client';

export default function CamerasPage() {
  const { t } = useTranslation();

  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: getCameras,
    refetchInterval: 10000,
  });

  if (cameras.length === 0) {
    return (
      <div className="max-w-7xl mx-auto px-4 py-12 text-center text-gray-500">
        {t('cameras.no_cameras')}
      </div>
    );
  }

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
      <h1 className="text-xl font-semibold text-gray-900 mb-6">
        {t('cameras.title')}
      </h1>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {cameras.map((cam) => (
          <Link
            key={cam.id}
            to={`/?camera=${cam.id}`}
            className="bg-white rounded-lg shadow p-4 hover:shadow-md transition-shadow min-h-[44px]"
          >
            <div className="flex items-center justify-between">
              <h3 className="font-medium text-gray-900">{cam.name}</h3>
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
            <p className="text-sm text-gray-500 mt-1">{cam.id}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
