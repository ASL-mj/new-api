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
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Button,
  Input,
  InputNumber,
  Switch,
  Tag,
  Typography,
  Descriptions,
} from '@douyinfe/semi-ui';
import {
  API,
  getCurrencyConfig,
  renderQuota,
  showError,
} from '../../../../helpers';
import {
  displayAmountToQuota,
  quotaToDisplayAmount,
} from '../../../../helpers/channelQuota';

const { Text } = Typography;

const MAX_KEY_NAME_LENGTH = 128;

const normalizeQuotaValue = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
};

// 统一的密钥编辑弹窗：支持只改名称、只改限额，或同时修改。
// 名称仅是展示别名，不参与鉴权，也不改变真实密钥与启用状态。
const ChannelKeyEditModal = ({
  visible,
  onCancel,
  channel,
  record,
  onSaved,
}) => {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [limitEnabled, setLimitEnabled] = useState(false);
  const [quota, setQuota] = useState(0);
  const [amount, setAmount] = useState(0);
  const [saving, setSaving] = useState(false);
  const currencyConfig = getCurrencyConfig();
  const isTokensDisplay = currencyConfig.type === 'TOKENS';

  useEffect(() => {
    if (!visible || !record) {
      return;
    }
    const initialQuota = Math.round(normalizeQuotaValue(record.quota_limit));
    setName(record.key_name || '');
    setLimitEnabled(initialQuota > 0);
    setQuota(initialQuota);
    setAmount(Number(quotaToDisplayAmount(initialQuota).toFixed(6)));
  }, [visible, record]);

  const handleAmountChange = (value) => {
    const nextAmount = normalizeQuotaValue(value);
    setAmount(nextAmount);
    setQuota(displayAmountToQuota(nextAmount));
  };

  const handleQuotaChange = (value) => {
    const nextQuota = Math.round(normalizeQuotaValue(value));
    setQuota(nextQuota);
    setAmount(Number(quotaToDisplayAmount(nextQuota).toFixed(6)));
  };

  const used = Number(record?.quota_limit_used || 0);
  const remaining = limitEnabled ? Math.max(0, quota - used) : null;

  const handleSave = async () => {
    const trimmedName = name.trim();
    if (trimmedName.length > MAX_KEY_NAME_LENGTH) {
      showError(
        t('密钥名称过长，最多 ${count} 个字符').replace(
          '${count}',
          MAX_KEY_NAME_LENGTH,
        ),
      );
      return;
    }
    if (limitEnabled && quota <= 0) {
      showError(t('开启限额后，限额数值必须大于 0'));
      return;
    }
    setSaving(true);
    try {
      const res = await API.put(
        `/api/channel/${channel.id}/key-usages/${encodeURIComponent(record.key_fingerprint)}/config`,
        {
          key_name: trimmedName,
          quota_limit: limitEnabled ? quota : 0,
        },
      );
      if (!res.data.success) {
        showError(res.data.message || t('保存失败'));
        return;
      }
      onSaved && (await onSaved());
    } catch (error) {
      showError(error?.message || t('保存失败'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      title={
        <div className='flex items-center gap-2'>
          <span>{t('编辑密钥')}</span>
          {record?.key_mask && (
            <Text code size='small'>
              {record.key_mask}
            </Text>
          )}
        </div>
      }
      visible={visible}
      onCancel={onCancel}
      closeOnEsc
      size='small'
      footer={
        <div className='flex justify-end gap-2'>
          <Button onClick={onCancel} disabled={saving}>
            {t('取消')}
          </Button>
          <Button
            type='primary'
            theme='solid'
            loading={saving}
            onClick={handleSave}
          >
            {t('保存')}
          </Button>
        </div>
      }
    >
      {record && (
        <div className='flex flex-col gap-4 py-2'>
          <Descriptions
            size='small'
            row
            data={[
              {
                key: t('密钥序号'),
                value: `#${Number(record.index) + 1}`,
              },
              {
                key: t('当前启用状态'),
                value:
                  record.status === 1 ? (
                    <Tag color='green' size='small'>
                      {t('已启用')}
                    </Tag>
                  ) : (
                    <Tag color='red' size='small'>
                      {t('已禁用')}
                    </Tag>
                  ),
              },
              {
                key: t('当前已用额度'),
                value: renderQuota(used),
              },
              {
                key: t('剩余额度'),
                value: remaining == null ? '∞' : renderQuota(remaining),
              },
            ]}
          />

          <div className='flex flex-col gap-1'>
            <Text size='small'>{t('密钥名称')}</Text>
            <Input
              value={name}
              placeholder={`${t('密钥')} #${Number(record.index) + 1}`}
              maxLength={MAX_KEY_NAME_LENGTH}
              showClear
              onChange={(value) => setName(value)}
            />
            <Text type='tertiary' size='small'>
              {t('名称仅用于展示，留空时列表显示密钥序号，不影响真实密钥。')}
            </Text>
          </div>

          <div className='flex items-center justify-between'>
            <Text size='small'>{t('启用限额')}</Text>
            <Switch
              checked={limitEnabled}
              onChange={(checked) => setLimitEnabled(checked)}
            />
          </div>

          {limitEnabled && (
            <div className='flex flex-col gap-2'>
              <Text type='tertiary' size='small'>
                {t('填写 0 表示无限额，修改限额不会改变当前启用状态。')}
              </Text>
              {isTokensDisplay ? (
                <div className='flex flex-col gap-1'>
                  <Text size='small'>{t('原生额度')}</Text>
                  <InputNumber
                    value={quota}
                    min={0}
                    precision={0}
                    step={1}
                    style={{ width: '100%' }}
                    onChange={handleQuotaChange}
                  />
                </div>
              ) : (
                <>
                  <Text size='small'>{t('金额')}</Text>
                  <InputNumber
                    value={amount}
                    prefix={currencyConfig.symbol}
                    min={0}
                    precision={6}
                    step={0.000001}
                    style={{ width: '100%' }}
                    onChange={handleAmountChange}
                  />
                </>
              )}
            </div>
          )}
        </div>
      )}
    </Modal>
  );
};

export default ChannelKeyEditModal;
