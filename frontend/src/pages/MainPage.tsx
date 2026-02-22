import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  getCameras,
  searchText,
} from '../api/client';
import type { SearchResult } from '../api/types';
import { defaultTimeRange } from '../utils/time';
import ResultCard from '../components/ResultCard';
import VideoPlayerModal from '../components/VideoPlayerModal';
import useVideoPlayer from '../hooks/useVideoPlayer';
import useSearchHistory from '../hooks/useSearchHistory';

const RESULTS_PER_PAGE = 16;
const SEARCH_FETCH_LIMIT = 200;

export default function MainPage() {
  const { t } = useTranslation();
  const { state: videoState, openVideo, closeVideo } = useVideoPlayer();

  // Camera list
  const { data: cameras = [] } = useQuery({
    queryKey: ['cameras'],
    queryFn: getCameras,
  });

  // Filter state
  const [defaults] = useState(defaultTimeRange);
  const [selectedCameras, setSelectedCameras] = useState<string[]>([]);
  const [startDate, setStartDate] = useState(defaults.startDate);
  const [endDate, setEndDate] = useState(defaults.endDate);
  const [startTime, setStartTime] = useState(defaults.startTime);
  const [endTime, setEndTime] = useState(defaults.endTime);

  // Search state
  const [query, setQuery] = useState('');
  const [searching, setSearching] = useState(false);
  const [results, setResults] = useState<SearchResult[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [showHistory, setShowHistory] = useState(false);
  const { history, addQuery, clearHistory } = useSearchHistory();

  // Pagination
  const [page, setPage] = useState(0);
  const resultsRef = useRef<HTMLDivElement>(null);

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

  const handleSearch = async () => {
    if (!query.trim()) return;

    setSearching(true);
    try {
      const trimmed = query.trim();
      const st = startDate
        ? `${startDate}T${startTime || '00:00'}:00`
        : undefined;
      const et = endDate
        ? `${endDate}T${endTime || '23:59'}:00`
        : undefined;
      const res = await searchText({
        query: trimmed,
        camera_ids: selectedCameras.length > 0 ? selectedCameras : undefined,
        start_time: st,
        end_time: et,
        limit: SEARCH_FETCH_LIMIT,
      });
      setResults(res.results);
      setPage(0);
      setSearchQuery(res.query);
      addQuery(trimmed);
    } catch {
      setResults([]);
    } finally {
      setSearching(false);
    }
  };

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-8">
      {/* Search */}
      <section className="bg-white rounded-lg shadow p-4 sm:p-6 space-y-4">
        <h2 className="text-lg font-semibold text-gray-900">
          {t('search.title')}
        </h2>

        {/* Camera selector */}
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-2">
            {t('search.cameras')}
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

        {/* Date & time range */}
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('search.start_date')}
            </label>
            <div className="flex gap-2">
              <input
                type="date"
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="flex-1 px-3 py-2 border rounded-md text-sm"
              />
              <input
                type="time"
                value={startTime}
                onChange={(e) => setStartTime(e.target.value)}
                className="w-28 px-3 py-2 border rounded-md text-sm"
              />
            </div>
          </div>
          <div className="flex-1">
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('search.end_date')}
            </label>
            <div className="flex gap-2">
              <input
                type="date"
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                className="flex-1 px-3 py-2 border rounded-md text-sm"
              />
              <input
                type="time"
                value={endTime}
                onChange={(e) => setEndTime(e.target.value)}
                className="w-28 px-3 py-2 border rounded-md text-sm"
              />
            </div>
          </div>
        </div>

        {/* Search input */}
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="relative flex-1">
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              onFocus={() => setShowHistory(true)}
              onBlur={() => setShowHistory(false)}
              placeholder={t('search.placeholder')}
              className="w-full px-4 py-2.5 border rounded-md text-sm min-h-[44px]"
            />
            {showHistory && history.length > 0 && (
              <div className="absolute z-10 left-0 right-0 mt-1 bg-white border rounded-md shadow-lg max-h-60 overflow-y-auto">
                {history.map((q, i) => (
                  <button
                    key={i}
                    type="button"
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => { setQuery(q); setShowHistory(false); }}
                    className="w-full text-left px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 truncate"
                  >
                    {q}
                  </button>
                ))}
                <button
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => { clearHistory(); setShowHistory(false); }}
                  className="w-full text-left px-4 py-2 text-xs text-red-500 hover:bg-red-50 border-t"
                >
                  {t('search.clear_history')}
                </button>
              </div>
            )}
          </div>
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
      {results.length > 0 && (() => {
        const totalPages = Math.ceil(results.length / RESULTS_PER_PAGE);
        const pagedResults = results.slice(
          page * RESULTS_PER_PAGE,
          (page + 1) * RESULTS_PER_PAGE,
        );
        return (
          <section ref={resultsRef} className="space-y-3">
            <p className="text-sm text-gray-600">
              {t('search.results_count', {
                count: results.length,
                query: searchQuery,
              })}
            </p>
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
              {pagedResults.map((r, i) => (
                <ResultCard
                  key={r.frame_id}
                  result={r}
                  rank={page * RESULTS_PER_PAGE + i + 1}
                  onPlayVideo={openVideo}
                />
              ))}
            </div>
            {totalPages > 1 && (
              <div className="flex items-center justify-center gap-4 pt-2">
                <button
                  onClick={() => {
                    setPage((p) => p - 1);
                    resultsRef.current?.scrollIntoView({ behavior: 'smooth' });
                  }}
                  disabled={page === 0}
                  className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
                >
                  {t('pagination.previous', 'Previous')}
                </button>
                <span className="text-sm text-gray-600">
                  {t('pagination.page_of', {
                    defaultValue: 'Page {{page}} of {{total}}',
                    page: page + 1,
                    total: totalPages,
                  })}
                </span>
                <button
                  onClick={() => {
                    setPage((p) => p + 1);
                    resultsRef.current?.scrollIntoView({ behavior: 'smooth' });
                  }}
                  disabled={page >= totalPages - 1}
                  className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border rounded-md hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
                >
                  {t('pagination.next', 'Next')}
                </button>
              </div>
            )}
          </section>
        );
      })()}

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
