/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import React, { useEffect, useState } from 'react';
import { Banner, Button, Input, InputNumber } from '@douyinfe/semi-ui';
import { Save } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showSuccess } from '../../../helpers';
import { H3_PRICE_KEY, validateH3Pricing } from '../../../helpers/h3Task';

const H3PricingSettings = ({ options, refresh }) => {
  const { t } = useTranslation();
  const [rule, setRule] = useState({});
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const raw = options?.[H3_PRICE_KEY];
  const registered = raw !== undefined;
  let configured = false;
  try {
    validateH3Pricing(typeof raw === 'string' ? JSON.parse(raw || '{}') : raw);
    configured = true;
  } catch {
    // Blank or partial persisted rules must never appear as configured pricing.
  }
  useEffect(() => {
    try {
      const parsed =
        typeof raw === 'string' ? JSON.parse(raw || '{}') : raw || {};
      if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object')
        throw new Error();
      setRule(parsed);
      setError('');
    } catch {
      setRule({});
      setError(t('价格规则加载失败'));
    }
  }, [raw, t]);
  const fields = [
    ['base_quota', t('基础额度（quota）'), 1],
    ['duration_base_seconds', t('时长基数（秒）'), 0.01],
    ['768P', t('768P 分辨率倍率'), 0.01],
    ['2K', t('2K 分辨率倍率'), 0.01],
    ['image_reference_ratio_per_image', t('每张图片附加倍率'), 0],
    ['video_reference_ratio_per_second', t('每秒参考视频附加倍率'), 0],
    ['audio_reference_ratio_per_audio', t('每段参考音频附加倍率'), 0],
  ];
  const update = (key, value) => {
    setRule((current) =>
      ['768P', '2K'].includes(key)
        ? {
            ...current,
            resolution_ratios: { ...current.resolution_ratios, [key]: value },
          }
        : { ...current, [key]: value },
    );
    setError('');
  };
  const save = async () => {
    if (!registered || saving) return;
    try {
      const data = validateH3Pricing(rule);
      setSaving(true);
      const res = await API.put(
        '/api/option/',
        { key: H3_PRICE_KEY, value: JSON.stringify(data) },
        { skipErrorHandler: true },
      );
      if (!res.data.success) throw new Error(res.data.message || t('保存失败'));
      showSuccess(t('保存成功'));
      await refresh?.();
    } catch (e) {
      setError(t(e.response?.data?.message || e.message || '保存失败'));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div style={{ maxWidth: 700 }}>
      {!configured && (
        <Banner type='warning' description={t('H3 定价配置尚未就绪')} />
      )}
      <div className='grid grid-cols-1 sm:grid-cols-2 gap-4 py-4'>
        <div>
          <label htmlFor='h3-price-version' className='block mb-2'>
            {t('价格版本')}
          </label>
          <Input
            id='h3-price-version'
            disabled={!registered || saving}
            value={rule.version || ''}
            onChange={(value) => update('version', value)}
          />
        </div>
        {fields.map(([key, label, min]) => (
          <div key={key}>
            <label htmlFor={'h3-price-' + key} className='block mb-2'>
              {label}
            </label>
            <InputNumber
              id={'h3-price-' + key}
              disabled={!registered || saving}
              value={
                ['768P', '2K'].includes(key)
                  ? rule.resolution_ratios?.[key]
                  : rule[key]
              }
              min={min}
              step={key === 'base_quota' ? 1 : 0.01}
              onChange={(value) => update(key, value)}
              style={{ width: '100%' }}
            />
          </div>
        ))}
      </div>
      {error && <Banner type='danger' description={error} />}
      <div className='flex justify-end mt-3'>
        <Button
          icon={<Save size={16} />}
          type='primary'
          theme='solid'
          disabled={!registered}
          loading={saving}
          onClick={save}
        >
          {t('保存 H3 规则')}
        </Button>
      </div>
    </div>
  );
};
export default H3PricingSettings;
