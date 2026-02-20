import { useState, useEffect, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  getSettings,
  updateSettings,
  getModelInfo,
  switchModel,
  getCameras,
  startProcess,
  streamProcessStatus,
  getNVRStatus,
} from '../api/client';
import type { NVRStatusResponse } from '../api/client';
import type { SettingsMap, ProgressEvent } from '../api/types';

interface FieldDef {
  key: string;
  label: string;
  hint: string;
  type: 'float' | 'int' | 'bool' | 'string' | 'password';
  step?: number;
  min?: number;
  max?: number;
}

const generalFields: FieldDef[] = [
  { key: 'general.system_name', label: 'settings.system_name', hint: 'settings.system_name_hint', type: 'string' },
];

const searchFields: FieldDef[] = [
  { key: 'search.min_score', label: 'settings.min_score', hint: 'settings.min_score_hint', type: 'float', step: 0.01, min: 0, max: 1 },
  { key: 'search.default_limit', label: 'settings.default_limit', hint: 'settings.default_limit_hint', type: 'int', min: 1, max: 500 },
];

const extractionFields: FieldDef[] = [
  { key: 'extraction.time_interval_sec', label: 'settings.time_interval', hint: 'settings.time_interval_hint', type: 'int', min: 1, max: 3600 },
  { key: 'extraction.output_quality', label: 'settings.output_quality', hint: 'settings.output_quality_hint', type: 'int', min: 1, max: 100 },
  { key: 'extraction.dedup_enabled', label: 'settings.dedup_enabled', hint: 'settings.dedup_enabled_hint', type: 'bool' },
  { key: 'extraction.dedup_phash_threshold', label: 'settings.dedup_threshold', hint: 'settings.dedup_threshold_hint', type: 'int', min: 0, max: 64 },
];

const clipFields: FieldDef[] = [
  { key: 'clip.batch_size', label: 'settings.batch_size', hint: 'settings.batch_size_hint', type: 'int', min: 1, max: 256 },
];

const nvrFields: FieldDef[] = [
  { key: 'nvr.ip', label: 'settings.nvr_ip', hint: 'settings.nvr_ip_hint', type: 'string' },
  { key: 'nvr.rtsp_port', label: 'settings.nvr_rtsp_port', hint: 'settings.nvr_rtsp_port_hint', type: 'int', min: 1, max: 65535 },
  { key: 'nvr.username', label: 'settings.nvr_username', hint: 'settings.nvr_username_hint', type: 'string' },
  { key: 'nvr.password', label: 'settings.nvr_password', hint: 'settings.nvr_password_hint', type: 'password' },
];

type ModelSwitchPhase = 'confirm' | 'loading_model' | 'reprocessing' | 'done' | 'error';

export default function SettingsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SettingsMap>({});
  const [defaults, setDefaults] = useState<SettingsMap>({});
  const [flash, setFlash] = useState<string | null>(null);
  const autoSaveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [selectedPreset, setSelectedPreset] = useState<string>('');

  // NVR status
  const [nvrStatus, setNvrStatus] = useState<NVRStatusResponse | null>(null);
  const [nvrChecking, setNvrChecking] = useState(false);

  // Model switch dialog state
  const [modelDialogOpen, setModelDialogOpen] = useState(false);
  const [modelSwitchPhase, setModelSwitchPhase] = useState<ModelSwitchPhase>('confirm');
  const [modelSwitchError, setModelSwitchError] = useState<string | null>(null);
  const [processEvents, setProcessEvents] = useState<ProgressEvent[]>([]);
  const cleanupRef = useRef<(() => void) | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
  });

  const { data: modelData } = useQuery({
    queryKey: ['clipModel'],
    queryFn: getModelInfo,
  });

  useEffect(() => {
    if (modelData) {
      setSelectedPreset(modelData.preset);
    }
  }, [modelData]);

  useEffect(() => {
    if (data) {
      setForm(data.settings);
      setDefaults(data.defaults);
    }
  }, [data]);

  const checkNVR = useCallback(async () => {
    setNvrChecking(true);
    try {
      const result = await getNVRStatus();
      setNvrStatus(result);
    } catch {
      setNvrStatus({ status: 'error', error: 'Request failed' });
    } finally {
      setNvrChecking(false);
    }
  }, []);

  // Check NVR status on mount
  useEffect(() => {
    checkNVR();
  }, [checkNVR]);

  const mutation = useMutation({
    mutationFn: updateSettings,
    onSuccess: (resp) => {
      setForm(resp.settings);
      setDefaults(resp.defaults);
      queryClient.setQueryData(['settings'], resp);
      setFlash(t('settings.saved'));
      setTimeout(() => setFlash(null), 3000);

      // Re-check NVR status after saving if any NVR field changed
      const nvrKeys = nvrFields.map((f) => f.key);
      const changed = nvrKeys.some((k) => resp.settings[k] !== data?.settings[k]);
      if (changed) checkNVR();
    },
  });

  const dirty = data && JSON.stringify(form) !== JSON.stringify(data.settings);

  // Auto-save: debounce 1.5s after last change
  useEffect(() => {
    if (!dirty) return;
    if (autoSaveTimer.current) clearTimeout(autoSaveTimer.current);
    autoSaveTimer.current = setTimeout(() => {
      mutation.mutate(form);
    }, 1500);
    return () => {
      if (autoSaveTimer.current) clearTimeout(autoSaveTimer.current);
    };
  }, [form, dirty]);

  // Cleanup SSE on unmount
  useEffect(() => {
    return () => {
      if (cleanupRef.current) cleanupRef.current();
    };
  }, []);

  const handleModelSwitch = useCallback(async () => {
    setModelSwitchPhase('loading_model');
    setModelSwitchError(null);
    setProcessEvents([]);

    try {
      // Phase 1: Switch model (downloads weights if needed, clears data)
      await switchModel(selectedPreset);
      queryClient.invalidateQueries({ queryKey: ['clipModel'] });

      // Phase 2: Reprocess all cameras
      setModelSwitchPhase('reprocessing');

      const cameras = await getCameras();
      if (cameras.length === 0) {
        setModelSwitchPhase('done');
        return;
      }

      const cameraIds = cameras.map((c) => c.id);
      const today = new Date().toISOString().split('T')[0];

      const resp = await startProcess({
        camera_ids: cameraIds,
        start_date: '2000-01-01',
        end_date: today,
      });

      if (resp.status === 'already_cached' || !resp.job_id) {
        // No videos to process
        setModelSwitchPhase('done');
        return;
      }

      // Stream process progress
      const cleanup = streamProcessStatus(
        resp.job_id,
        (event) => {
          setProcessEvents((prev) => [...prev, event]);
        },
        () => {
          setModelSwitchPhase('done');
          cleanupRef.current = null;
        },
      );
      cleanupRef.current = cleanup;
    } catch (err) {
      setModelSwitchError(err instanceof Error ? err.message : String(err));
      setModelSwitchPhase('error');
    }
  }, [selectedPreset, queryClient]);

  const closeModelDialog = () => {
    if (cleanupRef.current) {
      cleanupRef.current();
      cleanupRef.current = null;
    }
    setModelDialogOpen(false);
    setModelSwitchPhase('confirm');
    setModelSwitchError(null);
    setProcessEvents([]);
  };

  const handleChange = (key: string, value: number | boolean | string) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const handleReset = (key: string) => {
    if (defaults[key] !== undefined) {
      setForm((prev) => ({ ...prev, [key]: defaults[key] }));
    }
  };

  const handleSave = () => {
    if (autoSaveTimer.current) clearTimeout(autoSaveTimer.current);
    mutation.mutate(form);
  };

  if (isLoading) {
    return (
      <div className="max-w-3xl mx-auto px-4 py-12 text-center text-gray-500">
        {t('settings.loading')}
      </div>
    );
  }

  const isDefault = (key: string) => {
    return defaults[key] !== undefined && form[key] === defaults[key];
  };

  const formatDefault = (key: string, type: string) => {
    const val = defaults[key];
    if (val === undefined) return '';
    if (type === 'bool') return val ? 'true' : 'false';
    return String(val);
  };

  const renderResetLink = (fieldKey: string, fieldType: string) => {
    if (isDefault(fieldKey)) return null;
    const defaultLabel = t('settings.default', { value: formatDefault(fieldKey, fieldType) });
    return (
      <a
        href="#"
        onClick={(e) => { e.preventDefault(); handleReset(fieldKey); }}
        className="block text-xs text-blue-500 hover:text-blue-700 underline mt-1"
      >
        {defaultLabel}
      </a>
    );
  };

  const renderField = (field: FieldDef) => {
    const value = form[field.key];

    if (field.type === 'string' || field.type === 'password') {
      return (
        <label key={field.key} className="block py-3">
          <div className="flex items-center justify-between mb-1">
            <span className="text-sm font-medium text-gray-700">{t(field.label)}</span>
            <span className="text-xs text-gray-400">{t(field.hint)}</span>
          </div>
          <input
            type={field.type === 'password' ? 'password' : 'text'}
            value={(value as string) ?? ''}
            placeholder={String(defaults[field.key] ?? '')}
            onChange={(e) => handleChange(field.key, e.target.value)}
            className="w-full rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
          />
          {renderResetLink(field.key, field.type)}
        </label>
      );
    }
    if (field.type === 'bool') {
      return (
        <label key={field.key} className="flex items-center justify-between py-3">
          <div>
            <div className="text-sm font-medium text-gray-700">{t(field.label)}</div>
            <div className="text-xs text-gray-400">{t(field.hint)}</div>
            {renderResetLink(field.key, field.type)}
          </div>
          <input
            type="checkbox"
            checked={!!value}
            onChange={(e) => handleChange(field.key, e.target.checked)}
            className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
          />
        </label>
      );
    }
    return (
      <label key={field.key} className="block py-3">
        <div className="flex items-center justify-between mb-1">
          <span className="text-sm font-medium text-gray-700">{t(field.label)}</span>
          <span className="text-xs text-gray-400">{t(field.hint)}</span>
        </div>
        <input
          type="number"
          value={value ?? ''}
          step={field.step ?? 1}
          min={field.min}
          max={field.max}
          placeholder={formatDefault(field.key, field.type)}
          onChange={(e) => {
            const v = field.type === 'float' ? parseFloat(e.target.value) : parseInt(e.target.value, 10);
            if (!isNaN(v)) handleChange(field.key, v);
          }}
          className="w-full rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        />
        {renderResetLink(field.key, field.type)}
      </label>
    );
  };

  const renderCard = (title: string, fields: FieldDef[]) => (
    <div className="bg-white rounded-lg shadow p-5">
      <h2 className="text-base font-semibold text-gray-900 mb-3">{title}</h2>
      <div className="divide-y divide-gray-100">
        {fields.map(renderField)}
      </div>
    </div>
  );

  // Latest process event for display
  const latestEvent = processEvents.length > 0 ? processEvents[processEvents.length - 1] : null;
  const lastIndexingEvent = [...processEvents].reverse().find(
    (e) => e.stage === 'indexing' && e.frames_done !== undefined,
  );

  const isInProgress = modelSwitchPhase === 'loading_model' || modelSwitchPhase === 'reprocessing';

  const renderStepIcon = (stepPhase: ModelSwitchPhase, currentPhase: ModelSwitchPhase) => {
    const phaseOrder: ModelSwitchPhase[] = ['loading_model', 'reprocessing', 'done'];
    const stepIdx = phaseOrder.indexOf(stepPhase);
    const currentIdx = phaseOrder.indexOf(currentPhase);

    if (currentPhase === 'error') {
      // Mark the failed step with an X, prior steps as done
      if (stepIdx < currentIdx || currentIdx === -1) {
        return <span className="text-green-500">&#10003;</span>;
      }
      return <span className="text-red-500">&#10007;</span>;
    }

    if (currentIdx > stepIdx) {
      return <span className="text-green-500">&#10003;</span>;
    }
    if (currentIdx === stepIdx) {
      return (
        <svg className="animate-spin h-4 w-4 text-blue-500" viewBox="0 0 24 24" fill="none">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      );
    }
    return <span className="text-gray-300">&#9675;</span>;
  };

  return (
    <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
      <h1 className="text-xl font-semibold text-gray-900 mb-6">{t('settings.title')}</h1>

      {mutation.isError && (
        <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          {t('settings.error')}
        </div>
      )}

      <div className="space-y-5">
        {renderCard(t('settings.general_title'), generalFields)}
        <div className="bg-white rounded-lg shadow p-5">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-base font-semibold text-gray-900">{t('settings.nvr_title')}</h2>
            <div className="flex items-center gap-2">
              {nvrChecking ? (
                <span className="text-xs text-gray-400">{t('settings.nvr_checking')}</span>
              ) : nvrStatus?.status === 'connected' ? (
                <span className="text-xs px-2 py-0.5 rounded-full bg-green-100 text-green-700">
                  {t('settings.nvr_connected')}
                </span>
              ) : nvrStatus?.status === 'error' ? (
                <span className="text-xs px-2 py-0.5 rounded-full bg-red-100 text-red-700" title={nvrStatus.error}>
                  {t('settings.nvr_error')}
                </span>
              ) : nvrStatus?.status === 'not_configured' ? (
                <span className="text-xs px-2 py-0.5 rounded-full bg-gray-100 text-gray-500">
                  {t('settings.nvr_not_configured')}
                </span>
              ) : null}
              <button
                type="button"
                onClick={checkNVR}
                disabled={nvrChecking}
                className="text-xs text-blue-600 hover:text-blue-700 disabled:opacity-50"
              >
                {t('settings.nvr_test')}
              </button>
            </div>
          </div>
          {nvrStatus?.status === 'error' && nvrStatus.error && (
            <div className="mb-3 p-2 bg-red-50 border border-red-200 rounded text-xs text-red-600">
              {nvrStatus.error}
            </div>
          )}
          <div className="divide-y divide-gray-100">
            {nvrFields.map(renderField)}
          </div>
        </div>
        {renderCard(t('settings.search_title'), searchFields)}
        {renderCard(t('settings.extraction_title'), extractionFields)}
        {renderCard(t('settings.clip_title'), clipFields)}

        {/* CLIP Model selector */}
        <div className="bg-white rounded-lg shadow p-5">
          <h2 className="text-base font-semibold text-gray-900 mb-3">{t('settings.clip_model')}</h2>
          <div className="py-3">
            <div className="flex items-center justify-between mb-1">
              <span className="text-sm font-medium text-gray-700">{t('settings.clip_model')}</span>
              <span className="text-xs text-gray-400">{t('settings.clip_model_hint')}</span>
            </div>
            <select
              value={selectedPreset}
              onChange={(e) => setSelectedPreset(e.target.value)}
              className="w-full rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            >
              {modelData?.presets && Object.entries(modelData.presets).map(([key, name]) => (
                <option key={key} value={key}>{name}</option>
              ))}
            </select>
            {modelData && (
              <div className="mt-2 text-xs text-gray-400">
                {t('settings.clip_model_dim')}: {modelData.embedding_dim}
              </div>
            )}
          </div>
          <div className="flex items-center justify-end gap-3 mt-2">
            <button
              onClick={() => {
                setModelSwitchPhase('confirm');
                setModelSwitchError(null);
                setProcessEvents([]);
                setModelDialogOpen(true);
              }}
              disabled={selectedPreset === modelData?.preset}
              className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
            >
              {t('settings.clip_model_apply')}
            </button>
          </div>
        </div>
      </div>

      {/* Model switch dialog */}
      {modelDialogOpen && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
            {modelSwitchPhase === 'confirm' ? (
              <>
                <h3 className="text-lg font-semibold text-gray-900 mb-2">
                  {t('settings.clip_model_confirm_title')}
                </h3>
                <p className="text-sm text-gray-600 mb-4">
                  {t('settings.clip_model_confirm')}
                </p>
                <div className="flex justify-end gap-3">
                  <button
                    onClick={closeModelDialog}
                    className="px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 rounded hover:bg-gray-200 min-h-[44px]"
                  >
                    {t('cameras.cancel')}
                  </button>
                  <button
                    onClick={handleModelSwitch}
                    className="px-4 py-2 bg-red-600 text-white text-sm font-medium rounded hover:bg-red-700 min-h-[44px]"
                  >
                    {t('settings.clip_model_apply')}
                  </button>
                </div>
              </>
            ) : (
              <>
                <h3 className="text-lg font-semibold text-gray-900 mb-4">
                  {modelSwitchPhase === 'done'
                    ? t('settings.clip_model_switched')
                    : t('settings.clip_model_confirm_title')}
                </h3>

                {/* Step indicators */}
                <div className="space-y-3 mb-4">
                  {/* Step 1: Loading model */}
                  <div className="flex items-center gap-3">
                    <div className="flex-shrink-0 w-5 flex justify-center">
                      {renderStepIcon('loading_model', modelSwitchPhase)}
                    </div>
                    <span className={`text-sm ${
                      modelSwitchPhase === 'loading_model' ? 'text-gray-900 font-medium' : 'text-gray-500'
                    }`}>
                      {t('settings.clip_model_switching')}
                    </span>
                  </div>

                  {/* Step 2: Reprocessing */}
                  <div className="flex items-start gap-3">
                    <div className="flex-shrink-0 w-5 flex justify-center mt-0.5">
                      {renderStepIcon('reprocessing', modelSwitchPhase)}
                    </div>
                    <div className="flex-1 min-w-0">
                      <span className={`text-sm ${
                        modelSwitchPhase === 'reprocessing' ? 'text-gray-900 font-medium' : 'text-gray-500'
                      }`}>
                        {t('settings.clip_model_reprocessing')}
                      </span>
                      {modelSwitchPhase === 'reprocessing' && latestEvent && (
                        <div className="mt-1.5 text-xs text-gray-500 space-y-1">
                          {latestEvent.message && (
                            <p className="truncate">{latestEvent.message}</p>
                          )}
                          {lastIndexingEvent && lastIndexingEvent.frames_done !== undefined && lastIndexingEvent.frames_total !== undefined && (
                            <div>
                              <div className="w-full bg-gray-200 rounded-full h-1.5 mt-1">
                                <div
                                  className="bg-blue-500 h-1.5 rounded-full transition-all"
                                  style={{
                                    width: `${lastIndexingEvent.frames_total > 0
                                      ? Math.round((lastIndexingEvent.frames_done / lastIndexingEvent.frames_total) * 100)
                                      : 0}%`,
                                  }}
                                />
                              </div>
                              <p className="mt-0.5">
                                {lastIndexingEvent.frames_done} / {lastIndexingEvent.frames_total} {t('settings.clip_model_frames')}
                              </p>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                </div>

                {/* Error message */}
                {modelSwitchPhase === 'error' && modelSwitchError && (
                  <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
                    {modelSwitchError}
                  </div>
                )}

                {/* Footer buttons */}
                <div className="flex justify-end gap-3">
                  {modelSwitchPhase === 'done' || modelSwitchPhase === 'error' ? (
                    <button
                      onClick={closeModelDialog}
                      className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded hover:bg-blue-700 min-h-[44px]"
                    >
                      {t('settings.clip_model_close')}
                    </button>
                  ) : (
                    <div className="text-xs text-gray-400 italic">
                      {t('settings.clip_model_wait')}
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
        </div>
      )}

      <div className="flex items-center justify-end gap-3 mt-6">
        {flash && (
          <span className="text-sm text-green-600">{flash}</span>
        )}
        <button
          onClick={handleSave}
          disabled={!dirty || mutation.isPending}
          className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px]"
        >
          {mutation.isPending ? t('settings.saving') : t('settings.save')}
        </button>
      </div>
    </div>
  );
}
