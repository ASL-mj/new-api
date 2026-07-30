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
import { Card, Empty, Typography } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import {
  formatChartTime,
  formatLatency,
  formatPercentage,
} from '../../../../../helpers/performance';

const { Text } = Typography;

const CHART_CONFIG = { mode: 'desktop-browser' };

const ModelPerformanceCharts = ({ series = [], hours, t }) => {
  const chartId = useId();

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  const chartData = series.map((point) => ({
    ...point,
    time: formatChartTime(point.ts, hours),
  }));
  const chartSummary = t('趋势包含 {{count}} 个时间点', {
    count: chartData.length,
  });
  const ttftSpec = {
    type: 'line',
    data: [{ id: 'ttft', values: chartData }],
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
    point: { visible: false },
    line: { style: { lineWidth: 2 } },
    tooltip: {
      mark: {
        content: [
          {
            key: () => t('平均 TTFT'),
            value: (datum) => formatLatency(datum.avg_ttft_ms),
          },
        ],
      },
    },
  };
  const availabilitySpec = {
    type: 'line',
    data: [{ id: 'availability', values: chartData }],
    xField: 'time',
    yField: 'success_rate',
    axes: [
      { orient: 'bottom', label: { visible: true } },
      {
        orient: 'left',
        min: 0,
        max: 100,
        label: {
          visible: true,
          formatMethod: (value) => formatPercentage(Number(value)),
        },
      },
    ],
    point: { visible: false },
    line: { style: { lineWidth: 2 } },
    tooltip: {
      mark: {
        content: [
          {
            key: () => t('请求成功率'),
            value: (datum) => formatPercentage(datum.success_rate),
          },
        ],
      },
    },
  };

  const renderChart = (title, spec, label, descriptionId, describePoint) => (
    <Card title={title} bordered bodyStyle={{ padding: 8 }}>
      <Text type='tertiary' size='small'>
        {chartSummary}
      </Text>
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
    <section
      className='grid grid-cols-1 md:grid-cols-2 gap-3'
      aria-label={t('模型性能趋势')}
    >
      {renderChart(
        t('TTFT 延迟趋势'),
        ttftSpec,
        t('TTFT 延迟趋势，{{summary}}', { summary: chartSummary }),
        `${chartId}-ttft-description`,
        (point) => formatLatency(point.avg_ttft_ms),
      )}
      {renderChart(
        t('可用性趋势'),
        availabilitySpec,
        t('可用性趋势，{{summary}}', { summary: chartSummary }),
        `${chartId}-availability-description`,
        (point) => formatPercentage(point.success_rate),
      )}
    </section>
  );
};

export default ModelPerformanceCharts;
