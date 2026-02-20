import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  getCameras,
  startProcess,
  streamProcessStatus,
} from '../api/client';
import type { ProgressEvent } from '../api/types';

export default function ProcessPage() {
  const { t } = useTranslation();

  // Camera list
  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: getCameras,
  });

  // Form state
  const [selectedCameras, setSelectedCameras] = useState<string[]>([]);
  const [startDate, setStartDate] = useState(() => new Date().toISOString().split('T')[0]);
  const [endDate, setEndDate] = useState(() => new Date().toISOString().split('T')[0]);

  // Processing state
  const [processing, setProcessing] = useState(false);
  const [processReady, setProcessReady] = useState(false);
  const [events, setEvents] = useState<ProgressEvent[]>([]);
  const [progressPct, setProgressPct] = useState(0);

  // Auto-select all cameras on load
  useEffect(() => {
    if (cameras.length > 0 && selectedCameras.length === 0) {
      setSelectedCameras(cameras.map((c) => c.id));
    }
  }, [cameras, selectedCameras.length]);

  const toggleCamera = (id: string) => {
    setSelectedCameras((prev) =>
      prev.includes(id) ? prev.filter((c) => c !== id) : [...prev, id],
    );
  };

  const handleProcess = async () => {
    if (selectedCameras.length === 0 || !startDate) return;

    setProcessing(true);
    setProcessReady(false);
    setEvents([]);
    setProgressPct(0);

    try {
      const res = await startProcess({
        camera_ids: selectedCameras,
        start_date: startDate,
        end_date: endDate || startDate,
      });

      if (res.status === 'already_cached') {
        setProcessing(false);
        setProcessReady(true);
        return;
      }

      streamProcessStatus(
        res.job_id,
        (event) => {
          setEvents((prev) => [...prev, event]);
          if (event.frames_total && event.frames_total > 0) {
            setProgressPct(
              Math.round(((event.frames_done || 0) / event.frames_total) * 100),
            );
          }
        },
        () => {
          setProcessing(false);
          setProcessReady(true);
        },
      );
    } catch {
      setProcessing(false);
    }
  };

  const lastEvent = events[events.length - 1];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-8">
      <section className="bg-white rounded-lg shadow p-4 sm:p-6 space-y-4">
        <h2 className="text-lg font-semibold text-gray-900">
          {t('process.title')}
        </h2>

        {/* Camera selector */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            {t('process.cameras')}
          </label>
          <div className="flex flex-wrap gap-3">
            {cameras.map((cam) => (
              <label
                key={cam.id}
                className="flex items-center gap-2 min-h-[44px] cursor-pointer"
              >
                <input
                  type="checkbox"
                  checked={selectedCameras.includes(cam.id)}
                  onChange={() => toggleCamera(cam.id)}
                  className="w-4 h-4 rounded"
                />
                <span className="text-sm text-gray-800">{cam.name}</span>
              </label>
            ))}
            {cameras.length === 0 && (
              <span className="text-sm text-gray-400">
                {t('cameras.no_cameras')}
              </span>
            )}
          </div>
        </div>

        {/* Date picker */}
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('process.start_date')}
            </label>
            <input
              type="date"
              value={startDate}
              onChange={(e) => setStartDate(e.target.value)}
              className="w-full px-3 py-2 border rounded-md text-sm"
            />
          </div>
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('process.end_date')}
            </label>
            <input
              type="date"
              value={endDate}
              onChange={(e) => setEndDate(e.target.value)}
              className="w-full px-3 py-2 border rounded-md text-sm"
            />
          </div>
        </div>

        {/* Process button */}
        <div className="flex items-center gap-4">
          <button
            onClick={handleProcess}
            disabled={processing || selectedCameras.length === 0 || !startDate}
            className="px-5 py-2.5 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
          >
            {processing ? t('process.processing') : t('process.button')}
          </button>
          {processReady && (
            <span className="text-sm text-green-600 font-medium">
              {t('process.ready')}
            </span>
          )}
        </div>

        {/* Progress bar */}
        {processing && (
          <div className="space-y-2">
            <div className="w-full bg-gray-200 rounded-full h-2.5">
              <div
                className="bg-blue-600 h-2.5 rounded-full transition-all duration-300"
                style={{ width: `${progressPct}%` }}
              />
            </div>
            {lastEvent && (
              <p className="text-sm text-gray-600">
                {lastEvent.message}
              </p>
            )}
          </div>
        )}
      </section>
    </div>
  );
}
