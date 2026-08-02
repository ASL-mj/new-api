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

import React, { useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Card,
  Radio,
  RadioGroup,
  Spin,
  Typography,
} from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';

import { CHART_CONFIG } from '../../../constants';
import OpsMetricGrid from './OpsMetricGrid';

const WINDOW_SECONDS = {
  '1m': 60,
  '5m': 5 * 60,
  '30m': 30 * 60,
  '1h': 60 * 60,
};

const formatRate = (value) => Number(value || 0).toFixed(1);

const buildIdleWavePath = (phase, amplitude, baseline) =>
  Array.from({ length: 28 }, (_, index) => {
    const x = (index / 27) * 240;
    const y = baseline + Math.sin(index * 0.68 + phase) * amplitude;
    return `${index === 0 ? 'M' : 'L'}${x.toFixed(1)} ${y.toFixed(1)}`;
  }).join(' ');

const IdleWave = ({ phase, t }) => (
  <svg
    className='h-full w-full'
    viewBox='0 0 240 64'
    preserveAspectRatio='none'
    role='img'
    aria-label={t('暂无请求时的实时波形')}
  >
    <path
      d={buildIdleWavePath(phase, 4, 33)}
      fill='none'
      stroke='#2563eb'
      strokeLinecap='round'
      strokeWidth='2'
    />
    <path
      d={buildIdleWavePath(phase + 1.2, 3, 38)}
      fill='none'
      stroke='#93c5fd'
      strokeLinecap='round'
      strokeWidth='2'
    />
  </svg>
);

const getHealthMeta = (overview, t) => {
  if (!overview || Number(overview.request_count || 0) === 0) {
    return {
      label: t('待机'),
      detail: t('等待请求数据'),
      color: 'var(--semi-color-text-2)',
      score: 0,
      display: t('待机'),
    };
  }
  const score = Number(overview.health_score || 0);
  if (score >= 90) {
    return {
      label: t('运行稳定'),
      detail: t('系统健康'),
      color: 'var(--semi-color-success)',
      score,
      display: score,
    };
  }
  if (score >= 60) {
    return {
      label: t('需要关注'),
      detail: t('系统健康'),
      color: 'var(--semi-color-warning)',
      score,
      display: score,
    };
  }
  return {
    label: t('存在异常'),
    detail: t('系统健康'),
    color: 'var(--semi-color-danger)',
    score,
    display: score,
  };
};

const HealthRing = ({ health }) => {
  const radius = 45;
  const circumference = 2 * Math.PI * radius;
  const progress = Math.max(0, Math.min(100, health.score));
  return (
    <div className='flex flex-col items-center text-center'>
      <div className='relative h-28 w-28'>
        <svg className='h-full w-full -rotate-90' viewBox='0 0 112 112'>
          <circle
            cx='56'
            cy='56'
            r={radius}
            fill='none'
            stroke='var(--semi-color-fill-2)'
            strokeWidth='8'
          />
          <circle
            cx='56'
            cy='56'
            r={radius}
            fill='none'
            stroke={health.color}
            strokeWidth='8'
            strokeLinecap='round'
            strokeDasharray={circumference}
            strokeDashoffset={circumference * (1 - progress / 100)}
          />
        </svg>
        <div className='absolute inset-0 grid place-items-center'>
          <span className='text-xl font-bold tabular-nums'>
            {health.display}
          </span>
        </div>
      </div>
      <Typography.Text type='tertiary' size='small'>
        {health.detail}
      </Typography.Text>
      <span
        className='mt-1 text-sm font-semibold'
        style={{ color: health.color }}
      >
        {health.label}
      </span>
    </div>
  );
};

const OpsOverviewPanel = ({
  overview,
  overviewLoading,
  overviewError,
  waveWindow,
  setWaveWindow,
  openDetail,
  t,
}) => {
  const [idlePulse, setIdlePulse] = useState(0);

  useEffect(() => {
    const timer = window.setInterval(
      () => setIdlePulse((value) => value + 0.35),
      1000,
    );
    return () => window.clearInterval(timer);
  }, []);

  const points = useMemo(() => {
    const source = overview?.realtime || [];
    if (source.length === 0) return [];
    const latest = source[source.length - 1].ts;
    const cutoff = latest - WINDOW_SECONDS[waveWindow];
    return source.filter((point) => point.ts >= cutoff);
  }, [overview, waveWindow]);

  const chartSpec = useMemo(() => {
    const data = points.map((point) => ({
      time: point.ts,
      qps: point.qps || 0,
      tps: point.tps || 0,
    }));
    return {
      type: 'common',
      padding: 2,
      series: [
        {
          type: 'line',
          id: 'qps-series',
          data: { id: 'ops-realtime-qps', values: data },
          xField: 'time',
          yField: 'qps',
          line: { style: { stroke: '#2563eb', lineWidth: 2 } },
          point: { visible: false },
        },
        {
          type: 'line',
          id: 'tps-series',
          data: { id: 'ops-realtime-tps', values: data },
          xField: 'time',
          yField: 'tps',
          line: { style: { stroke: '#93c5fd', lineWidth: 2 } },
          point: { visible: false },
        },
      ],
      axes: [
        { orient: 'bottom', visible: false },
        { orient: 'left', seriesId: ['qps-series'], visible: false },
        { orient: 'right', seriesId: ['tps-series'], visible: false },
      ],
      crosshair: { xField: { visible: true } },
      tooltip: { visible: true },
    };
  }, [points]);

  const health = getHealthMeta(overview, t);

  return (
    <Card className='mb-3 !rounded-lg' bodyStyle={{ padding: 18 }}>
      {overviewError && (
        <Banner type='danger' description={overviewError} className='mb-3' />
      )}
      {overviewLoading && !overview ? (
        <div className='flex h-80 items-center justify-center'>
          <Spin />
        </div>
      ) : (
        <div className='grid grid-cols-1 gap-4 xl:grid-cols-[minmax(340px,5fr)_minmax(0,7fr)]'>
          <section
            className='grid min-h-[322px] grid-cols-1 gap-4 rounded-lg p-4 sm:grid-cols-[132px_minmax(0,1fr)]'
            style={{ background: 'var(--semi-color-fill-0)' }}
          >
            <div
              className='flex items-center justify-center border-b pb-4 sm:border-b-0 sm:border-r sm:pb-0 sm:pr-4'
              style={{ borderColor: 'var(--semi-color-border)' }}
            >
              <HealthRing health={health} />
            </div>

            <div className='flex min-w-0 flex-col justify-center'>
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <div
                  className='flex items-center gap-2 text-xs font-semibold'
                  style={{ color: 'var(--semi-color-text-2)' }}
                >
                  <i className='h-2 w-2 rounded-full bg-blue-500 shadow-[0_0_0_4px_rgba(59,130,246,0.12)]' />
                  {t('实时吞吐')}
                </div>
                <RadioGroup
                  type='button'
                  size='small'
                  value={waveWindow}
                  onChange={(event) => setWaveWindow(event.target.value)}
                >
                  {Object.keys(WINDOW_SECONDS).map((window) => (
                    <Radio key={window} value={window}>
                      {window}
                    </Radio>
                  ))}
                </RadioGroup>
              </div>

              <Typography.Text type='tertiary' size='small' className='mt-4'>
                {t('当前')}
              </Typography.Text>
              <div className='mt-1 flex flex-wrap items-baseline gap-x-5 gap-y-1'>
                <div>
                  <span className='text-2xl font-bold tabular-nums'>
                    {formatRate(overview?.qps?.current)}
                  </span>
                  <span className='ml-1 text-xs font-semibold text-slate-500'>
                    QPS
                  </span>
                </div>
                <div>
                  <span className='text-2xl font-bold tabular-nums'>
                    {formatRate(overview?.tps?.current)}
                  </span>
                  <span className='ml-1 text-xs font-semibold text-slate-500'>
                    TPS
                  </span>
                </div>
              </div>

              <div className='mt-4 grid grid-cols-2 gap-4 text-xs'>
                <div>
                  <Typography.Text type='tertiary' size='small'>
                    {t('峰值')}
                  </Typography.Text>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {formatRate(overview?.qps?.peak)} QPS
                  </div>
                  <div className='mt-0.5 font-semibold tabular-nums'>
                    {formatRate(overview?.tps?.peak)} TPS
                  </div>
                </div>
                <div>
                  <Typography.Text type='tertiary' size='small'>
                    {t('平均')}
                  </Typography.Text>
                  <div className='mt-1 font-semibold tabular-nums'>
                    {formatRate(overview?.qps?.average)} QPS
                  </div>
                  <div className='mt-0.5 font-semibold tabular-nums'>
                    {formatRate(overview?.tps?.average)} TPS
                  </div>
                </div>
              </div>

              <div className='relative mt-3 h-20 min-w-0'>
                {points.length ? (
                  <VChart spec={chartSpec} option={CHART_CONFIG} />
                ) : (
                  <div
                    className='h-full border-b'
                    style={{
                      borderColor: 'var(--semi-color-border)',
                    }}
                  >
                    <IdleWave phase={idlePulse} t={t} />
                  </div>
                )}
                {!points.length && (
                  <span
                    className='absolute bottom-1 left-0 text-[10px]'
                    style={{ color: 'var(--semi-color-text-2)' }}
                  >
                    {t('暂无请求 · 实时监测中')}
                  </span>
                )}
              </div>
              <div
                className='mt-1 flex items-center gap-4 text-[10px]'
                style={{ color: 'var(--semi-color-text-2)' }}
              >
                <span>
                  <i className='mr-1 inline-block h-0.5 w-4 bg-blue-600 align-middle' />
                  QPS
                </span>
                <span>
                  <i className='mr-1 inline-block h-0.5 w-4 bg-blue-300 align-middle' />
                  TPS
                </span>
              </div>
            </div>
          </section>

          <OpsMetricGrid overview={overview} openDetail={openDetail} t={t} />
        </div>
      )}
    </Card>
  );
};

export default OpsOverviewPanel;
