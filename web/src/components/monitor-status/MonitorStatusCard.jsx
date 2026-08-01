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

import React from 'react';
import { Card, Tag, Typography } from '@douyinfe/semi-ui';
import { Activity, Globe2 } from 'lucide-react';

import { getLobeHubIcon } from '../../helpers';
import MonitorTimeline from './MonitorTimeline';
import {
  formatMonitorLatencyValue,
  formatMonitorRefresh,
  getMonitorProviderIconName,
  getMonitorStatusMeta,
} from './monitorTimelineUtils';

const MetricPair = ({ primary, secondary, t }) => {
  const items = [
    { icon: <Activity size={14} />, label: t('模型延迟'), value: primary },
    { icon: <Globe2 size={14} />, label: t('端点 Ping'), value: secondary },
  ];
  return (
    <div className='mt-3 grid grid-cols-2 gap-2'>
      {items.map((item) => (
        <div
          key={item.label}
          className='rounded-lg border p-2.5'
          style={{
            background: 'var(--semi-color-fill-0)',
            borderColor: 'var(--semi-color-border)',
          }}
        >
          <div
            className='flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {item.icon}
            <span>{item.label}</span>
          </div>
          <div className='mt-1 truncate text-base font-bold tabular-nums'>
            {item.value}
            <span
              className='ml-1 text-xs font-normal'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              ms
            </span>
          </div>
        </div>
      ))}
    </div>
  );
};

const availabilityColor = (value) => {
  if (value == null) return 'var(--semi-color-text-2)';
  if (value >= 99) return 'var(--semi-color-success)';
  if (value >= 95) return 'var(--semi-color-warning)';
  return 'var(--semi-color-danger)';
};

const ProviderBadge = ({ group }) => {
  const types = group.channel_types || [];
  const provider = types.join(' / ') || '自定义渠道';
  const iconName = getMonitorProviderIconName(types);
  return (
    <div
      className='grid h-10 w-10 flex-none place-items-center rounded-xl'
      style={{
        color: 'var(--semi-color-primary)',
        background: 'var(--semi-color-primary-light-default)',
      }}
      aria-label={provider}
    >
      {getLobeHubIcon(iconName, 23)}
    </div>
  );
};

const MonitorStatusCard = ({ group, onOpen, t, refreshAfter }) => {
  const status = getMonitorStatusMeta(group.status);
  const availability = group.availability;
  const channelType = group.channel_types?.join(' / ') || t('自定义渠道');
  const open = () => onOpen(group);
  return (
    <Card
      className='h-full cursor-pointer !rounded-lg transition-shadow hover:shadow-md'
      bodyStyle={{ padding: 14 }}
      onClick={open}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') open();
      }}
      role='button'
      tabIndex={0}
    >
      <div className='flex items-start gap-3'>
        <ProviderBadge group={group} />
        <div className='min-w-0 flex-1'>
          <Typography.Title
            heading={6}
            ellipsis={{ showTooltip: true }}
            style={{ margin: 0 }}
          >
            {group.name}
          </Typography.Title>
          <div className='mt-1 flex min-w-0 items-center gap-1.5'>
            <Tag color='blue' size='small' shape='circle'>
              {channelType}
            </Tag>
            <span
              className='truncate font-mono text-xs'
              style={{ color: 'var(--semi-color-text-2)' }}
              title={group.primary_model}
            >
              {group.primary_model || '--'}
            </span>
          </div>
        </div>
        <Tag color={status.color} shape='circle'>
          {t(status.label)}
        </Tag>
      </div>

      <MetricPair
        primary={formatMonitorLatencyValue(group.current_latency_ms)}
        secondary={formatMonitorLatencyValue(group.current_ping_latency_ms)}
        t={t}
      />

      <div
        className='mt-3 flex items-end justify-between border-y py-3'
        style={{ borderColor: 'var(--semi-color-border)' }}
      >
        <span
          className='text-[11px] uppercase tracking-widest'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          {t('可用性')} ·{' '}
          {group.availability_days || group.AvailabilityDays || 30} {t('天')}
        </span>
        <span
          className='text-2xl font-bold leading-none tabular-nums'
          style={{ color: availabilityColor(availability) }}
        >
          {availability == null ? '--' : Number(availability).toFixed(2)}
          <small className='ml-0.5 text-base font-semibold'>%</small>
        </span>
      </div>

      <div className='mt-2'>
        <div
          className='mb-1.5 flex justify-between text-[10px] font-semibold uppercase tracking-widest'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          <span>{t('近 60 次记录')}</span>
          <span>{formatMonitorRefresh(refreshAfter)}</span>
        </div>
        <MonitorTimeline timeline={group.timeline} />
      </div>
    </Card>
  );
};

export default MonitorStatusCard;
