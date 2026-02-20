import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import type { CameraInfo, CreateCameraRequest, UpdateCameraRequest } from '../api/types';
import { createCamera, updateCamera, deleteCamera, downloadVideo, uploadVideos } from '../api/client';

// --- Shared modal backdrop ---

function ModalBackdrop({
  children,
  onClose,
}: {
  children: React.ReactNode;
  onClose: () => void;
}) {
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="bg-white rounded-lg shadow-xl w-full max-w-md"
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}

// --- Add Camera Modal ---

interface AddCameraModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export function AddCameraModal({ isOpen, onClose, onCreated }: AddCameraModalProps) {
  const { t } = useTranslation();
  const [id, setId] = useState('');
  const [name, setName] = useState('');
  const [type, setType] = useState<'local' | 'test'>('local');
  const [url, setUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (isOpen) {
      setId('');
      setName('');
      setType('local');
      setUrl('');
      setError('');
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      const req: CreateCameraRequest = { id, name, type };
      if (type === 'test' && url) {
        req.config = { url };
      }
      await createCamera(req);
      onCreated();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ModalBackdrop onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="px-6 py-4 border-b">
          <h2 className="text-lg font-semibold">{t('cameras.add_title')}</h2>
        </div>
        <div className="px-6 py-4 space-y-4">
          {error && (
            <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_id')}
            </label>
            <input
              type="text"
              value={id}
              onChange={(e) => setId(e.target.value)}
              pattern="[a-zA-Z0-9][a-zA-Z0-9_-]*"
              maxLength={64}
              required
              className="w-full border rounded px-3 py-2 text-sm"
              placeholder="my-camera-1"
            />
            <p className="text-xs text-gray-500 mt-1">{t('cameras.field_id_hint')}</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_name')}
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              className="w-full border rounded px-3 py-2 text-sm"
              placeholder="Front Door Camera"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_type')}
            </label>
            <select
              value={type}
              onChange={(e) => setType(e.target.value as 'local' | 'test')}
              className="w-full border rounded px-3 py-2 text-sm"
            >
              <option value="local">{t('cameras.type_local')}</option>
              <option value="test">{t('cameras.type_test')}</option>
            </select>
          </div>
          {type === 'test' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                {t('cameras.field_url')}
              </label>
              <input
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                className="w-full border rounded px-3 py-2 text-sm"
                placeholder="https://example.com/video.mp4"
              />
              <p className="text-xs text-gray-500 mt-1">{t('cameras.field_url_hint')}</p>
            </div>
          )}
        </div>
        <div className="px-6 py-4 border-t flex justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded min-h-[44px]"
          >
            {t('cameras.cancel')}
          </button>
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 text-sm text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50 min-h-[44px]"
          >
            {loading ? t('cameras.creating') : t('cameras.add')}
          </button>
        </div>
      </form>
    </ModalBackdrop>
  );
}

// --- Edit Camera Modal ---

interface EditCameraModalProps {
  isOpen: boolean;
  camera: CameraInfo | null;
  onClose: () => void;
  onUpdated: () => void;
}

export function EditCameraModal({ isOpen, camera, onClose, onUpdated }: EditCameraModalProps) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (isOpen && camera) {
      setName(camera.name);
      setUrl((camera.config?.url as string) || '');
      setError('');
    }
  }, [isOpen, camera]);

  if (!isOpen || !camera) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      const req: UpdateCameraRequest = { name };
      if (camera.type === 'test') {
        req.config = { url };
      }
      await updateCamera(camera.id, req);
      onUpdated();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ModalBackdrop onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="px-6 py-4 border-b">
          <h2 className="text-lg font-semibold">{t('cameras.edit_title')}</h2>
        </div>
        <div className="px-6 py-4 space-y-4">
          {error && (
            <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_id')}
            </label>
            <input
              type="text"
              value={camera.id}
              disabled
              className="w-full border rounded px-3 py-2 text-sm bg-gray-50 text-gray-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_type')}
            </label>
            <input
              type="text"
              value={t(`cameras.type_${camera.type}`)}
              disabled
              className="w-full border rounded px-3 py-2 text-sm bg-gray-50 text-gray-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_name')}
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              className="w-full border rounded px-3 py-2 text-sm"
            />
          </div>
          {camera.type === 'test' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                {t('cameras.field_url')}
              </label>
              <input
                type="url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                className="w-full border rounded px-3 py-2 text-sm"
                placeholder="https://example.com/video.mp4"
              />
            </div>
          )}
        </div>
        <div className="px-6 py-4 border-t flex justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded min-h-[44px]"
          >
            {t('cameras.cancel')}
          </button>
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 text-sm text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50 min-h-[44px]"
          >
            {loading ? t('cameras.saving') : t('cameras.save')}
          </button>
        </div>
      </form>
    </ModalBackdrop>
  );
}

// --- Delete Camera Dialog ---

interface DeleteCameraDialogProps {
  isOpen: boolean;
  camera: CameraInfo | null;
  onClose: () => void;
  onDeleted: () => void;
}

export function DeleteCameraDialog({ isOpen, camera, onClose, onDeleted }: DeleteCameraDialogProps) {
  const { t } = useTranslation();
  const [deleteData, setDeleteData] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (isOpen) {
      setDeleteData(false);
      setError('');
    }
  }, [isOpen]);

  if (!isOpen || !camera) return null;

  const handleDelete = async () => {
    setLoading(true);
    setError('');
    try {
      await deleteCamera(camera.id, deleteData);
      onDeleted();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ModalBackdrop onClose={onClose}>
      <div className="px-6 py-4 border-b">
        <h2 className="text-lg font-semibold text-red-600">{t('cameras.delete_title')}</h2>
      </div>
      <div className="px-6 py-4 space-y-4">
        {error && (
          <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>
        )}
        <p className="text-sm text-gray-700">
          {t('cameras.delete_confirm', { name: camera.name })}
        </p>
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={deleteData}
            onChange={(e) => setDeleteData(e.target.checked)}
            className="rounded"
          />
          {t('cameras.delete_data')}
        </label>
      </div>
      <div className="px-6 py-4 border-t flex justify-end gap-3">
        <button
          type="button"
          onClick={onClose}
          className="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded min-h-[44px]"
        >
          {t('cameras.cancel')}
        </button>
        <button
          type="button"
          onClick={handleDelete}
          disabled={loading}
          className="px-4 py-2 text-sm text-white bg-red-600 hover:bg-red-700 rounded disabled:opacity-50 min-h-[44px]"
        >
          {loading ? t('cameras.deleting') : t('cameras.delete')}
        </button>
      </div>
    </ModalBackdrop>
  );
}

// --- Download Video Modal ---

interface DownloadVideoModalProps {
  isOpen: boolean;
  camera: CameraInfo | null;
  onClose: () => void;
  onDownloaded: () => void;
}

export function DownloadVideoModal({ isOpen, camera, onClose, onDownloaded }: DownloadVideoModalProps) {
  const { t } = useTranslation();
  const [url, setUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState(false);

  useEffect(() => {
    if (isOpen && camera) {
      setUrl((camera.config?.url as string) || '');
      setError('');
      setSuccess(false);
    }
  }, [isOpen, camera]);

  if (!isOpen || !camera) return null;

  const handleDownload = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    setSuccess(false);
    try {
      await downloadVideo(camera.id, { url });
      setSuccess(true);
      onDownloaded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ModalBackdrop onClose={onClose}>
      <form onSubmit={handleDownload}>
        <div className="px-6 py-4 border-b">
          <h2 className="text-lg font-semibold">{t('cameras.download_title')}</h2>
        </div>
        <div className="px-6 py-4 space-y-4">
          {error && (
            <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>
          )}
          {success && (
            <div className="text-sm text-green-600 bg-green-50 rounded p-2">
              {t('cameras.download_success')}
            </div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              {t('cameras.field_url')}
            </label>
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              required
              className="w-full border rounded px-3 py-2 text-sm"
              placeholder="https://example.com/video.mp4"
            />
            <p className="text-xs text-gray-500 mt-1">{t('cameras.field_url_hint')}</p>
          </div>
        </div>
        <div className="px-6 py-4 border-t flex justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded min-h-[44px]"
          >
            {t('cameras.cancel')}
          </button>
          <button
            type="submit"
            disabled={loading}
            className="px-4 py-2 text-sm text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50 min-h-[44px]"
          >
            {loading ? t('cameras.downloading') : t('cameras.download')}
          </button>
        </div>
      </form>
    </ModalBackdrop>
  );
}

// --- Upload Video Modal ---

interface UploadVideoModalProps {
  isOpen: boolean;
  camera: CameraInfo | null;
  onClose: () => void;
  onUploaded: () => void;
}

export function UploadVideoModal({ isOpen, camera, onClose, onUploaded }: UploadVideoModalProps) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'files' | 'directory'>('files');
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  useEffect(() => {
    if (isOpen) {
      setMode('files');
      setSelectedFiles([]);
      setError('');
      setSuccess('');
    }
  }, [isOpen]);

  if (!isOpen || !camera) return null;

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = e.target.files;
    if (!files) return;
    const mp4Files = Array.from(files).filter(
      (f) => f.name.toLowerCase().endsWith('.mp4')
    );
    setSelectedFiles(mp4Files);
    setError('');
    setSuccess('');
  };

  const handleUpload = async () => {
    if (selectedFiles.length === 0) return;
    setLoading(true);
    setError('');
    setSuccess('');
    try {
      const result = await uploadVideos(camera.id, selectedFiles);
      setSuccess(t('cameras.upload_success', { count: result.paths.length }));
      setSelectedFiles([]);
      onUploaded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ModalBackdrop onClose={onClose}>
      <div>
        <div className="px-6 py-4 border-b">
          <h2 className="text-lg font-semibold">{t('cameras.upload_title')}</h2>
        </div>
        <div className="px-6 py-4 space-y-4">
          {error && (
            <div className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</div>
          )}
          {success && (
            <div className="text-sm text-green-600 bg-green-50 rounded p-2">{success}</div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              {t('cameras.upload_mode')}
            </label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => { setMode('files'); setSelectedFiles([]); }}
                className={`px-3 py-1.5 text-sm rounded ${
                  mode === 'files'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                }`}
              >
                {t('cameras.upload_mode_files')}
              </button>
              <button
                type="button"
                onClick={() => { setMode('directory'); setSelectedFiles([]); }}
                className={`px-3 py-1.5 text-sm rounded ${
                  mode === 'directory'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                }`}
              >
                {t('cameras.upload_mode_directory')}
              </button>
            </div>
          </div>
          <div>
            {mode === 'files' ? (
              <input
                key="files"
                type="file"
                accept=".mp4"
                multiple
                onChange={handleFileChange}
                className="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
              />
            ) : (
              <input
                key="directory"
                type="file"
                // @ts-expect-error webkitdirectory is not in React types
                webkitdirectory=""
                onChange={handleFileChange}
                className="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
              />
            )}
          </div>
          {selectedFiles.length > 0 && (
            <div>
              <p className="text-sm text-gray-600 mb-1">
                {t('cameras.upload_selected', { count: selectedFiles.length })}
              </p>
              <ul className="text-xs text-gray-500 max-h-32 overflow-y-auto space-y-0.5">
                {selectedFiles.map((f, i) => (
                  <li key={i} className="truncate">{f.name}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
        <div className="px-6 py-4 border-t flex justify-end gap-3">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 rounded min-h-[44px]"
          >
            {t('cameras.cancel')}
          </button>
          <button
            type="button"
            onClick={handleUpload}
            disabled={loading || selectedFiles.length === 0}
            className="px-4 py-2 text-sm text-white bg-blue-600 hover:bg-blue-700 rounded disabled:opacity-50 min-h-[44px]"
          >
            {loading ? t('cameras.uploading') : t('cameras.upload')}
          </button>
        </div>
      </div>
    </ModalBackdrop>
  );
}
