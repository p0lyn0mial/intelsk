import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getSettings, updateSettings } from '../api/client';
import type { SettingsMap } from '../api/types';

interface FieldDef {
  key: string;
  label: string;
  hint: string;
  type: 'float' | 'int' | 'bool';
  step?: number;
  min?: number;
  max?: number;
}

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

export default function SettingsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<SettingsMap>({});
  const [flash, setFlash] = useState<string | null>(null);

  const { data: settings, isLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
    onSuccess: (data: SettingsMap) => setForm(data),
  });

  const mutation = useMutation({
    mutationFn: updateSettings,
    onSuccess: (data) => {
      setForm(data);
      queryClient.setQueryData(['settings'], data);
      setFlash(t('settings.saved'));
      setTimeout(() => setFlash(null), 3000);
    },
  });

  const handleChange = (key: string, value: number | boolean) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const handleSave = () => {
    mutation.mutate(form);
  };

  const dirty = settings && JSON.stringify(form) !== JSON.stringify(settings);

  if (isLoading) {
    return (
      <div className="max-w-3xl mx-auto px-4 py-12 text-center text-gray-500">
        {t('settings.loading')}
      </div>
    );
  }

  const renderField = (field: FieldDef) => {
    const value = form[field.key];
    if (field.type === 'bool') {
      return (
        <label key={field.key} className="flex items-center justify-between py-3">
          <div>
            <div className="text-sm font-medium text-gray-700">{t(field.label)}</div>
            <div className="text-xs text-gray-400">{t(field.hint)}</div>
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
          onChange={(e) => {
            const v = field.type === 'float' ? parseFloat(e.target.value) : parseInt(e.target.value, 10);
            if (!isNaN(v)) handleChange(field.key, v);
          }}
          className="w-full rounded border border-gray-300 px-3 py-1.5 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
        />
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

  return (
    <div className="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-gray-900">{t('settings.title')}</h1>
        <div className="flex items-center gap-3">
          {flash && (
            <span className="text-sm text-green-600">{flash}</span>
          )}
          <button
            onClick={handleSave}
            disabled={!dirty || mutation.isPending}
            className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {mutation.isPending ? t('settings.saving') : t('settings.save')}
          </button>
        </div>
      </div>

      {mutation.isError && (
        <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          {t('settings.error')}
        </div>
      )}

      <div className="space-y-5">
        {renderCard(t('settings.search_title'), searchFields)}
        {renderCard(t('settings.extraction_title'), extractionFields)}
        {renderCard(t('settings.clip_title'), clipFields)}
      </div>
    </div>
  );
}
