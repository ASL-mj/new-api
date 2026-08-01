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

import React, { useEffect, useId } from 'react';
import { Avatar, Card, Empty, Typography } from '@douyinfe/semi-ui';
import { Activity, AlertTriangle, HeartPulse } from 'lucide-react';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import {
  buildPerformanceChartSeries,
  formatChartTime,
  formatLatency,
  formatPercentage,
  countPerformanceIncidents,
  getSuccessRateColor,
  MODEL_PERFORMANCE_HOURS,
} from '../../../../../helpers/performance';

const { Text } = Typography;

const CHART_CONFIG = { mode: 'desktop-browser' };

const getAvailabilityAxisMin = (points) => {
  const values = points
    .map((point) => Number(point.success_rate))
    .filter(Number.isFinite);
  if (values.length === 0) return 95;
  const minimum = Math.min(...values);
  if (minimum >= 95) return 95;
  if (minimum >= 90) return 90;
  return Math.max(0, Math.floor((minimum - 5) / 10) * 10);
};

const SectionHeading = ({ icon: Icon, title, description, accent }) => (
  <div className='mb-2 flex items-start justify-between gap-3'>
    <div className='flex min-w-0 items-center gap-2'>
      <Avatar size='small' color='green' className='shrink-0 shadow-md'>
        <Icon size={16} />
      </Avatar>
      <div className='min-w-0'>
        <Text strong>{title}</Text>
        <div className='text-xs text-gray-500'>{description}</div>
      </div>
    </div>
    {accent}
  </div>
);

const ModelPerformanceCharts = ({ series = [], t }) => {
  const chartId = useId();

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  const chartSeries = buildPerformanceChartSeries(series);
  const latencyData = chartSeries.latency.map((point) => ({
    ...point,
    time: formatChartTime(point.ts, MODEL_PERFORMANCE_HOURS),
  }));
  const availabilityData = chartSeries.availability.map((point) => ({
    ...point,
    time: formatChartTime(point.ts, MODEL_PERFORMANCE_HOURS),
  }));
  const incidents = countPerformanceIncidents(series);
  const ttftSpec = {
    type: 'line',
    data: [{ id: `${chartId}-ttft`, values: latencyData }],
    xField: 'time',
    yField: 'avg_ttft_ms',
    axes: [
      { orient: 'bottom', label: { visible: true } },
      {
        orient: 'left',
        label: {
          visible: true,
          formatMethod: (value) => formatLatency(Number(value)),
        },
      },
    ],
    smooth: true,
    point: {
      visible: true,
      style: { size: 5, stroke: '#ffffff', lineWidth: 1.5 },
    },
    line: { style: { lineWidth: 2 }, connectNulls: false },
    tooltip: {
      mark: {
        title: { value: (datum) => datum.time },
        content: [
          {
            key: t('平均 TTFT'),
            value: (datum) => formatLatency(datum.avg_ttft_ms),
          },
        ],
      },
    },
  };
  const availabilitySpec = {
    type: 'line',
    data: [{ id: `${chartId}-availability`, values: availabilityData }],
    xField: 'time',
    yField: 'success_rate',
    axes: [
      { orient: 'bottom', label: { visible: true } },
      {
        orient: 'left',
        min: getAvailabilityAxisMin(availabilityData),
        max: 100,
        label: {
          visible: true,
          formatMethod: (value) => formatPercentage(Number(value)),
        },
      },
    ],
    smooth: true,
    point: {
      visible: true,
      style: {
        size: 5,
        stroke: '#ffffff',
        lineWidth: 1.5,
        fill: (datum) => getSuccessRateColor(datum.success_rate),
      },
    },
    line: { style: { lineWidth: 2, stroke: '#10b981' }, connectNulls: false },
    tooltip: {
      mark: {
        title: { value: (datum) => datum.time },
        content: [
          {
            key: t('请求成功率'),
            value: (datum) => formatPercentage(datum.success_rate),
          },
        ],
      },
    },
  };

  const renderChart = (
    heading,
    spec,
    label,
    descriptionId,
    chartData,
    describePoint,
  ) => (
    <Card bordered bodyStyle={{ padding: 12 }}>
      {heading}
      <span id={descriptionId} className='sr-only'>
        {chartData.length > 0
          ? chartData
              .map((point) => `${point.time}: ${describePoint(point)}`)
              .join('; ')
          : t('无趋势数据')}
      </span>
      {chartData.length > 0 ? (
        <div
          role='img'
          aria-label={label}
          aria-describedby={descriptionId}
          style={{ height: 220, marginTop: 4 }}
        >
          <VChart spec={spec} option={CHART_CONFIG} />
        </div>
      ) : (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={t('无趋势数据')}
          style={{ padding: '56px 0' }}
        />
      )}
    </Card>
  );

  return (
    <section className='grid grid-cols-1 gap-3' aria-label={t('模型性能趋势')}>
      {renderChart(
        <SectionHeading
          icon={Activity}
          title={t('TTFT 延迟趋势')}
          description={t('平均首 Token 延迟')}
        />,
        ttftSpec,
        t('TTFT 延迟趋势（最近 24 小时）'),
        `${chartId}-ttft-description`,
        latencyData,
        (point) => formatLatency(point.avg_ttft_ms),
      )}
      {renderChart(
        <SectionHeading
          icon={HeartPulse}
          title={t('可用性趋势')}
          description={
            incidents > 0
              ? t('最近 24 小时共有 {{count}} 个异常桶', { count: incidents })
              : t('最近 24 小时无异常事件')
          }
          accent={
            incidents > 0 ? (
              <div
                className='flex shrink-0 items-center gap-1 text-xs font-medium'
                style={{ color: '#ef4444' }}
              >
                <AlertTriangle size={14} />
                {t('{{count}} 起事件', { count: incidents })}
              </div>
            ) : null
          }
        />,
        availabilitySpec,
        t('可用率（最近 24 小时）'),
        `${chartId}-availability-description`,
        availabilityData,
        (point) => formatPercentage(point.success_rate),
      )}
    </section>
  );
};

export default ModelPerformanceCharts;
