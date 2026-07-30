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
import { Table, Tag, Typography } from '@douyinfe/semi-ui';
import {
  formatLatency,
  formatPercentage,
  formatTps,
} from '../../../../../helpers/performance';

const { Text } = Typography;

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
      width: 104,
      render: formatPercentage,
    },
  ];

  return (
    <section
      aria-labelledby='model-performance-group-title'
      style={{ minWidth: 0, maxWidth: '100%', overflow: 'hidden' }}
    >
      <Text strong id='model-performance-group-title'>
        {t('分组性能')}
      </Text>
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
          style={{ minWidth: 608 }}
        />
      </div>
    </section>
  );
};

export default ModelPerformanceGroupTable;
