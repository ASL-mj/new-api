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
import { Avatar, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { Activity } from 'lucide-react';
import {
  formatChartTime,
  formatLatency,
  formatPercentage,
  formatTps,
  getSuccessRateColor,
  getUptimeBarHeight,
  normalizeHourlyPerformanceSeries,
} from '../../../../../helpers/performance';

const { Text } = Typography;

const SuccessRateBars = ({ series, rate, t }) => {
  const points = normalizeHourlyPerformanceSeries(series || []);
  return (
    <div className='flex min-w-[170px] items-center gap-2' title={t('成功率')}>
      <div className='flex h-5 min-w-0 flex-1 items-end gap-px'>
        {points.map((point) => {
          return (
            <span
              key={point.ts}
              className='min-w-0 flex-1 rounded-sm'
              title={
                point.has_data
                  ? `${formatChartTime(point.ts, 24)} · ${formatPercentage(point.success_rate)} · ${point.request_count} ${t('次请求')}`
                  : `${formatChartTime(point.ts, 24)} · ${t('暂无请求数据')}`
              }
              style={{
                height: getUptimeBarHeight(
                  point.success_rate,
                  point.has_data,
                ),
                backgroundColor: point.has_data
                  ? getSuccessRateColor(point.success_rate)
                  : 'var(--semi-color-fill-1)',
              }}
            />
          );
        })}
      </div>
      <span
        className='w-14 shrink-0 text-right font-mono text-xs font-semibold tabular-nums'
        style={{ color: getSuccessRateColor(rate) }}
      >
        {formatPercentage(rate)}
      </span>
    </div>
  );
};

const ModelPerformanceGroupTable = ({ groups, t }) => {
  const columns = [
    {
      title: t('分组'),
      dataIndex: 'group',
      width: 120,
      render: (group) => (
        <Tag color='white' size='small' shape='circle'>
          {group}
        </Tag>
      ),
    },
    {
      title: t('TPS'),
      dataIndex: 'avg_tps',
      width: 112,
      render: formatTps,
    },
    {
      title: t('平均 TTFT'),
      dataIndex: 'avg_ttft_ms',
      width: 132,
      render: formatLatency,
    },
    {
      title: t('平均总延迟'),
      dataIndex: 'avg_latency_ms',
      width: 140,
      render: formatLatency,
    },
    {
      title: t('成功率'),
      dataIndex: 'success_rate',
      width: 210,
      render: (rate, group) => (
        <SuccessRateBars series={group.series} rate={rate} t={t} />
      ),
    },
  ];

  return (
    <section
      aria-label={t('分组性能')}
      style={{ minWidth: 0, maxWidth: '100%', overflow: 'hidden' }}
    >
      <div className='flex items-center gap-2'>
        <Avatar size='small' color='green' className='shadow-md'>
          <Activity size={16} />
        </Avatar>
        <div>
          <Text strong>{t('分组性能')}</Text>
          <div className='text-xs text-gray-500'>
            {t('仅统计真实 Relay 请求')}
          </div>
        </div>
      </div>
      <div
        style={{
          width: '100%',
          maxWidth: '100%',
          minWidth: 0,
          overflowX: 'auto',
          WebkitOverflowScrolling: 'touch',
          paddingBottom: 4,
          marginTop: 8,
        }}
      >
        <Table
          dataSource={groups}
          columns={columns}
          rowKey='group'
          pagination={false}
          size='small'
          bordered={false}
          style={{ minWidth: 714 }}
        />
      </div>
    </section>
  );
};

export default ModelPerformanceGroupTable;
