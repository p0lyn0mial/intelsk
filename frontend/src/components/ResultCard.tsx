import { useTranslation } from 'react-i18next';
import { format } from 'date-fns';
import type { SearchResult } from '../api/types';
import PlayButtonOverlay from './PlayButtonOverlay';

interface Props {
  result: SearchResult;
  rank: number;
  onPlayVideo: (result: SearchResult) => void;
}

export default function ResultCard({ result, rank, onPlayVideo }: Props) {
  const { t } = useTranslation();

  let formattedTime = result.timestamp;
  try {
    formattedTime = format(new Date(result.timestamp), 'HH:mm:ss');
  } catch {
    // keep raw timestamp
  }

  return (
    <div className="bg-white rounded-lg shadow overflow-hidden">
      <div className="relative aspect-video bg-gray-100">
        <img
          src={result.frame_url}
          alt={`${result.camera_id} ${result.timestamp}`}
          className="w-full h-full object-cover"
          loading="lazy"
        />
        {result.source_video_url && (
          <PlayButtonOverlay onClick={() => onPlayVideo(result)} />
        )}
        <div className="absolute top-1 right-1 bg-black/70 text-white text-xs px-1.5 py-0.5 rounded font-mono">
          #{rank}
        </div>
      </div>
      <div className="p-2 text-sm space-y-0.5">
        <div className="flex justify-between items-center">
          <span className="font-medium text-gray-900">{result.camera_id}</span>
          <span className="text-gray-500 text-xs">{formattedTime}</span>
        </div>
        <div className="flex justify-between items-center text-xs text-gray-500">
          <span>{t('results.score')}: {result.score.toFixed(3)}</span>
        </div>
      </div>
    </div>
  );
}
