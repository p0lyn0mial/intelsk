import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  getCameras,
  startProcess,
  streamProcessStatus,
  searchText,
} from '../api/client';
import type { ProgressEvent, SearchResult } from '../api/types';
import ResultCard from '../components/ResultCard';
import VideoPlayerModal from '../components/VideoPlayerModal';
import useVideoPlayer from '../hooks/useVideoPlayer';

export default function MainPage() {
  const { t } = useTranslation();
  const { state: videoState, openVideo, closeVideo } = useVideoPlayer();

  // Camera list
  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: getCameras,
  });

  // Form state
  const [selectedCameras, setSelectedCameras] = useState<string[]>([]);
  const [startDate, setStartDate] = useState('');
  const [endDate, setEndDate] = useState('');

  // Processing state
  const [processing, setProcessing] = useState(false);
  const [processReady, setProcessReady] = useState(false);
  const [events, setEvents] = useState<ProgressEvent[]>([]);
  const [progressPct, setProgressPct] = useState(0);

  // Search state
  const [query, setQuery] = useState('');
  const [searching, setSearching] = useState(false);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searchQuery, setSearchQuery] = useState('');

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

  const handleSearch = async () => {
    if (!query.trim()) return;

    setSearching(true);
    try {
      const res = await searchText({
        query: query.trim(),
        camera_ids: selectedCameras.length > 0 ? selectedCameras : undefined,
        limit: 40,
      });
      setResults(res.results);
      setSearchQuery(res.query);
    } catch {
      setResults([]);
    } finally {
      setSearching(false);
    }
  };

  const lastEvent = events[events.length - 1];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-8">
      {/* Step 1: Process */}
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

      {/* Step 2: Search */}
      <section className="bg-white rounded-lg shadow p-4 sm:p-6 space-y-4">
        <h2 className="text-lg font-semibold text-gray-900">
          {t('search.title')}
        </h2>

        <div className="flex flex-col sm:flex-row gap-3">
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            placeholder={t('search.placeholder')}
            className="flex-1 px-4 py-2.5 border rounded-md text-sm min-h-[44px]"
          />
          <button
            onClick={handleSearch}
            disabled={searching || !query.trim()}
            className="px-5 py-2.5 bg-blue-600 text-white rounded-md text-sm font-medium hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
          >
            {searching ? t('search.searching') : t('search.button')}
          </button>
        </div>
      </section>

      {/* Results */}
      {results.length > 0 && (
        <section className="space-y-3">
          <p className="text-sm text-gray-600">
            {t('search.results_count', {
              count: results.length,
              query: searchQuery,
            })}
          </p>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
            {results.map((r, i) => (
              <ResultCard
                key={r.frame_id}
                result={r}
                rank={i + 1}
                onPlayVideo={openVideo}
              />
            ))}
          </div>
        </section>
      )}

      {searchQuery && results.length === 0 && !searching && (
        <p className="text-sm text-gray-500 text-center py-8">
          {t('search.no_results')}
        </p>
      )}

      {/* Video Player */}
      <VideoPlayerModal
        isOpen={videoState.isOpen}
        onClose={closeVideo}
        sourceVideoUrl={videoState.sourceVideoUrl}
        seekOffsetSec={videoState.seekOffsetSec}
        cameraName={videoState.cameraName}
        timestamp={videoState.timestamp}
      />
    </div>
  );
}
