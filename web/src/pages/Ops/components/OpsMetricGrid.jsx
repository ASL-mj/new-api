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
import { Tooltip, Typography } from '@douyinfe/semi-ui';
import { Info } from 'lucide-react';

const percent = (value) => `${Number(value || 0).toFixed(2)}%`;
const INFINITE_LATENCY_BOUND_MS = 60000;
const latency = (value) => {
  if (value == null) return '--';
  const numericValue = Number(value);
  if (!Number.isFinite(numericValue)) return '--';
  if (numericValue >= Number.MAX_SAFE_INTEGER) {
    return `>${INFINITE_LATENCY_BOUND_MS.toLocaleString()}`;
  }
  return numericValue.toLocaleString();
};
const count = (value) => Number(value || 0).toLocaleString();
const rate = (value) => Number(value || 0).toFixed(1);

const percentileRows = (data, t) => [
  {
    label: 'P95 / P90',
    value: `${latency(data?.p95_ms)} / ${latency(data?.p90_ms)} ms`,
  },
  {
    label: `P50 / ${t('平均')}`,
    value: `${latency(data?.p50_ms)} / ${latency(data?.average_ms)} ms`,
  },
  { label: t('最大值'), value: `${latency(data?.max_ms)} ms` },
];

export const buildOpsMetricItems = (overview, t) => {
  const upstreamTotal = Number(overview?.upstream_errors?.total || 0);
  const excluded =
    Number(overview?.upstream_errors?.status_429 || 0) +
    Number(overview?.upstream_errors?.status_529 || 0);
  return [
    {
      key: 'requests',
      title: t('所有请求'),
      tip: t('统计选定时间范围内的全部请求、Token、平均 QPS 与 TPS。'),
      layout: 'rows',
      rows: [
        { label: t('请求数'), value: count(overview?.request_count) },
        { label: t('Token 数'), value: count(overview?.token_count) },
        { label: t('平均 QPS'), value: rate(overview?.qps?.average) },
        { label: t('平均 TPS'), value: rate(overview?.tps?.average) },
      ],
    },
    {
      key: 'sla',
      title: t('SLA 请求'),
      tip: t('SLA 统计排除业务限制后的有效请求质量。'),
      value: percent(overview?.sla),
      tone: Number(overview?.sla || 0) >= 99 ? 'success' : 'danger',
      progress: Number(overview?.sla || 0),
      rows: [
        { label: t('异常数'), value: count(overview?.error_count) },
        { label: t('SLA 样本'), value: count(overview?.sla_sample_count) },
      ],
    },
    {
      key: 'errors',
      title: t('错误请求'),
      tip: t('统计请求失败比例，并区分普通错误与业务额度限制。'),
      value: percent(overview?.error_rate),
      tone: Number(overview?.error_rate || 0) > 1 ? 'danger' : 'success',
      rows: [
        { label: t('错误数'), value: count(overview?.error_count) },
        {
          label: t('业务限制'),
          value: count(overview?.business_limited_count),
        },
      ],
    },
    {
      key: 'duration',
      title: t('请求时长'),
      tip: t('请求从进入 NewAPI 到完成响应的耗时分布。'),
      value: latency(overview?.duration?.p99_ms),
      unit: 'ms P99',
      rows: percentileRows(overview?.duration, t),
    },
    {
      key: 'ttft',
      title: 'TTFT',
      tip: t('Time To First Token，首个输出 Token 到达前的等待时间。'),
      value: latency(overview?.ttft?.p99_ms),
      unit: 'ms P99',
      rows: percentileRows(overview?.ttft, t),
    },
    {
      key: 'upstream',
      title: t('上游错误请求'),
      tip: t('统计上游服务错误，并单独列出 429 与 529。'),
      value: percent(overview?.upstream_error_rate),
      tone: upstreamTotal > 0 ? 'warning' : 'success',
      rows: [
        { label: t('排除 429/529'), value: count(upstreamTotal - excluded) },
        { label: '429 / 529', value: count(excluded) },
      ],
    },
  ];
};

const toneColor = (tone) => {
  if (tone === 'danger') return 'var(--semi-color-danger)';
  if (tone === 'warning') return 'var(--semi-color-warning)';
  if (tone === 'success') return 'var(--semi-color-success)';
  return 'var(--semi-color-text-0)';
};

const OpsMetricCard = ({ metric, openDetail, t }) => {
  const open = () => openDetail(metric);
  return (
    <div
      className='flex min-h-36 cursor-pointer flex-col rounded-lg border p-3.5 transition-shadow hover:shadow-sm'
      style={{
        background: 'var(--semi-color-fill-0)',
        borderColor: 'var(--semi-color-border)',
      }}
      onClick={open}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') open();
      }}
      role='button'
      tabIndex={0}
      aria-label={`${metric.title}${t('明细')}`}
    >
      <div className='flex items-center justify-between gap-2'>
        <div
          className='flex items-center gap-1.5 text-xs font-medium'
          style={{ color: 'var(--semi-color-text-2)' }}
        >
          <span>{metric.title}</span>
          <Tooltip content={metric.tip} position='top'>
            <span className='inline-flex cursor-help'>
              <Info size={13} />
            </span>
          </Tooltip>
        </div>
        <span className='text-xs font-medium text-blue-600'>{t('明细')}</span>
      </div>

      {metric.value !== undefined && (
        <div className='mt-3 flex items-baseline gap-1'>
          <span
            className='text-2xl font-bold leading-none tabular-nums'
            style={{ color: toneColor(metric.tone) }}
          >
            {metric.value}
          </span>
          {metric.unit && (
            <span
              className='text-[10px] font-semibold'
              style={{ color: 'var(--semi-color-text-2)' }}
            >
              {metric.unit}
            </span>
          )}
        </div>
      )}

      {metric.progress !== undefined && (
        <div
          className='mt-3 h-1.5 overflow-hidden rounded-full'
          style={{ background: 'var(--semi-color-fill-2)' }}
        >
          <div
            className='h-full rounded-full transition-[width]'
            style={{
              width: `${Math.max(0, Math.min(100, metric.progress))}%`,
              background: toneColor(metric.tone),
            }}
          />
        </div>
      )}

      <div
        className={`${metric.value === undefined ? 'mt-4' : 'mt-3'} space-y-1.5`}
      >
        {metric.rows.map((row) => (
          <div
            key={row.label}
            className='flex items-center justify-between gap-3 text-xs'
          >
            <Typography.Text type='tertiary' size='small'>
              {row.label}
            </Typography.Text>
            <span className='text-right font-semibold tabular-nums'>
              {row.value}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
};

const OpsMetricGrid = ({ overview, openDetail, t, className = '' }) => (
  <div
    className={`grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3 ${className}`}
  >
    {buildOpsMetricItems(overview, t).map((metric) => (
      <OpsMetricCard
        key={metric.key}
        metric={metric}
        openDetail={openDetail}
        t={t}
      />
    ))}
  </div>
);

export default OpsMetricGrid;
