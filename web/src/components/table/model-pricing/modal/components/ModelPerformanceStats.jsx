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
import { Card, Typography } from '@douyinfe/semi-ui';
import { Gauge, HeartPulse, Timer } from 'lucide-react';
import {
  countPerformanceIncidents,
  formatLatency,
  formatPercentage,
  formatTps,
  getSuccessRateColor,
} from '../../../../../helpers/performance';

const { Text } = Typography;

const ModelPerformanceStats = ({ overall, t }) => {
  const incidentCount = countPerformanceIncidents(overall.series || []);
  const stats = [
    {
      icon: Gauge,
      label: t('平均 TPS'),
      value: formatTps(overall.avg_tps),
    },
    {
      icon: Timer,
      label: t('平均总延迟'),
      value: formatLatency(overall.avg_latency_ms),
    },
    {
      icon: HeartPulse,
      label: t('成功率'),
      value: formatPercentage(overall.success_rate),
      valueColor: getSuccessRateColor(overall.success_rate),
      hint: t('最近 24 小时 {{count}} 个异常时间段', {
        count: incidentCount,
      }),
    },
  ];

  return (
    <div
      className='grid gap-2'
      style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))' }}
      aria-label={t('模型性能概览')}
    >
      {stats.map((stat) => {
        const Icon = stat.icon;
        return (
          <Card key={stat.label} bordered bodyStyle={{ padding: '12px' }}>
            <div className='flex items-center gap-1.5'>
              <Icon size={14} className='text-gray-500' />
              <Text type='tertiary' size='small'>
                {stat.label}
              </Text>
            </div>
            <div
              className='mt-1 break-words text-base font-semibold'
              style={{ color: stat.valueColor }}
            >
              {stat.value}
            </div>
            {stat.hint && (
              <Text type='tertiary' size='small'>
                {stat.hint}
              </Text>
            )}
          </Card>
        );
      })}
    </div>
  );
};

export default ModelPerformanceStats;
