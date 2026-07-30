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
import {
  formatLatency,
  formatPercentage,
  formatTps,
} from '../../../../../helpers/performance';

const { Text } = Typography;

const ModelPerformanceStats = ({ overall, t }) => {
  const stats = [
    { label: t('平均 TPS'), value: formatTps(overall.avg_tps) },
    { label: t('平均总延迟'), value: formatLatency(overall.avg_latency_ms) },
    { label: t('成功率'), value: formatPercentage(overall.success_rate) },
  ];

  return (
    <div
      className='grid gap-2'
      style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))' }}
      aria-label={t('模型性能概览')}
    >
      {stats.map((stat) => (
        <Card key={stat.label} bordered bodyStyle={{ padding: '12px' }}>
          <Text type='tertiary' size='small'>
            {stat.label}
          </Text>
          <div className='mt-1 text-base font-semibold break-words'>
            {stat.value}
          </div>
        </Card>
      ))}
    </div>
  );
};

export default ModelPerformanceStats;
