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

import React, { useMemo } from 'react';
import { Card } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';

import { CHART_CONFIG } from '../../../constants';

const OpsTrendPanel = ({ overview, t }) => {
  const points = overview?.realtime || [];
  const spec = useMemo(() => {
    const values = points.flatMap((point) => {
      const time = new Date(point.ts * 1000).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
      });
      return [
        { time, metric: t('SLA'), value: point.sla || 0 },
        { time, metric: t('错误率'), value: point.error_rate || 0 },
      ];
    });
    return {
      type: 'line',
      data: { values },
      xField: 'time',
      yField: 'value',
      seriesField: 'metric',
      color: ['#16a34a', '#dc2626'],
      point: { visible: false },
      legends: { visible: true, orient: 'top' },
      axes: [
        { orient: 'bottom', label: { visible: points.length <= 12 } },
        {
          orient: 'left',
          min: 0,
          max: 100,
          title: { visible: true, text: '%' },
        },
      ],
      tooltip: { visible: true },
      padding: { left: 44, right: 16, top: 12, bottom: 26 },
    };
  }, [points, t]);

  return (
    <Card
      className='h-full !rounded-lg'
      title={t('请求质量趋势')}
      bodyStyle={{ padding: 12 }}
    >
      <div className='h-72'>
        {points.length ? (
          <VChart spec={spec} option={CHART_CONFIG} />
        ) : (
          <div
            className='flex h-full items-center justify-center text-sm'
            style={{ color: 'var(--semi-color-text-2)' }}
          >
            {t('所选范围暂无趋势数据')}
          </div>
        )}
      </div>
    </Card>
  );
};

export default OpsTrendPanel;
