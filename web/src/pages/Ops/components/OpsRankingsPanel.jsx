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
import { Banner, Card, Tag } from '@douyinfe/semi-ui';

import CardTable from '../../../components/common/ui/CardTable';

const OpsRankingsPanel = ({ rankings, rankingsLoading, rankingsError, t }) => {
  const columns = [
    {
      title: t('模型 / 渠道'),
      key: 'target',
      render: (_, row) => (
        <div className='min-w-0'>
          <div className='truncate font-medium' title={row.model_name}>
            {row.model_name || '--'}
          </div>
          <div
            className='truncate text-xs'
            style={{ color: 'var(--semi-color-text-2)' }}
            title={row.channel_name}
          >
            {row.channel_name || `#${row.channel_id}`} ·{' '}
            {row.group || 'default'}
          </div>
        </div>
      ),
    },
    {
      title: t('请求'),
      dataIndex: 'request_count',
      key: 'requests',
      width: 70,
    },
    {
      title: t('成功率'),
      dataIndex: 'success_rate',
      key: 'success_rate',
      width: 90,
      render: (value) => `${Number(value || 0).toFixed(2)}%`,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (value) => (
        <Tag
          color={
            value === 'operational'
              ? 'green'
              : value === 'degraded'
                ? 'orange'
                : 'red'
          }
          shape='circle'
          size='small'
        >
          {value === 'operational'
            ? t('正常')
            : value === 'degraded'
              ? t('降级')
              : t('异常')}
        </Tag>
      ),
    },
  ];

  return (
    <Card
      className='h-full !rounded-lg'
      title={t('模型与渠道排行')}
      bodyStyle={{ padding: 12 }}
    >
      {rankingsError && (
        <Banner type='danger' description={rankingsError} className='mb-2' />
      )}
      <CardTable
        columns={columns}
        dataSource={(rankings || []).slice(0, 8)}
        rowKey={(row) => `${row.channel_id}-${row.group}-${row.model_name}`}
        loading={rankingsLoading}
        pagination={false}
        scroll={{ x: 'max-content' }}
        empty={t('暂无排行数据')}
        size='small'
      />
    </Card>
  );
};

export default OpsRankingsPanel;
