import { useTranslation } from 'react-i18next';

interface Props {
  onClick: () => void;
}

export default function PlayButtonOverlay({ onClick }: Props) {
  const { t } = useTranslation();

  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className="absolute inset-0 flex items-center justify-center bg-black/0 hover:bg-black/40 transition-colors group"
      aria-label={t('video.play')}
    >
      <div className="w-12 h-12 rounded-full bg-white/80 flex items-center justify-center opacity-0 group-hover:opacity-100 [@media(hover:none)]:opacity-60 transition-opacity">
        <svg className="w-6 h-6 text-gray-900 ml-1" fill="currentColor" viewBox="0 0 24 24">
          <path d="M8 5v14l11-7z" />
        </svg>
      </div>
    </button>
  );
}
