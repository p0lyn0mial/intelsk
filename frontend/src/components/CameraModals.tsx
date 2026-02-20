import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import type { CameraInfo, CreateCameraRequest, UpdateCameraRequest } from '../api/types';
import { createCamera, updateCamera, deleteCamera, uploadVideos } from '../api/client';

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
  const [transcode, setTranscode] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (isOpen) {
      setId('');
      setName('');
      setTranscode(true);
      setError('');
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      const req: CreateCameraRequest = { id, name, type: 'local', config: { transcode } };
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
          <label className="flex items-center gap-2 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={transcode}
              onChange={(e) => setTranscode(e.target.checked)}
              className="rounded"
            />
            {t('cameras.transcode')}
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
  const [transcode, setTranscode] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (isOpen && camera) {
      setName(camera.name);
      setTranscode(camera.config?.transcode !== false);
      setError('');
    }
  }, [isOpen, camera]);

  if (!isOpen || !camera) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    try {
      const req: UpdateCameraRequest = { name, config: { transcode } };
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
          <label className="flex items-center gap-2 text-sm text-gray-700">
            <input
              type="checkbox"
              checked={transcode}
              onChange={(e) => setTranscode(e.target.checked)}
              className="rounded"
            />
            {t('cameras.transcode')}
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
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  useEffect(() => {
    if (isOpen) {
      setMode('files');
      setSelectedFiles([]);
      setUploadProgress(0);
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
    setUploadProgress(0);
    setError('');
    setSuccess('');
    try {
      const result = await uploadVideos(camera.id, selectedFiles, (loaded, total) => {
        setUploadProgress(Math.round((loaded / total) * 100));
      });
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
                disabled={loading}
                onClick={() => { setMode('files'); setSelectedFiles([]); }}
                className={`px-3 py-1.5 text-sm rounded disabled:opacity-50 ${
                  mode === 'files'
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                }`}
              >
                {t('cameras.upload_mode_files')}
              </button>
              <button
                type="button"
                disabled={loading}
                onClick={() => { setMode('directory'); setSelectedFiles([]); }}
                className={`px-3 py-1.5 text-sm rounded disabled:opacity-50 ${
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
                disabled={loading}
                onChange={handleFileChange}
                className="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
              />
            ) : (
              <input
                key="directory"
                type="file"
                // @ts-expect-error webkitdirectory is not in React types
                webkitdirectory=""
                disabled={loading}
                onChange={handleFileChange}
                className="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100 disabled:opacity-50"
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
          {loading && (
            <div className="space-y-1">
              <div className="w-full bg-gray-200 rounded-full h-2.5">
                <div
                  className="bg-blue-600 h-2.5 rounded-full transition-all duration-300"
                  style={{ width: `${uploadProgress}%` }}
                />
              </div>
              <p className="text-xs text-gray-500 text-right">{uploadProgress}%</p>
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
