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
import { Button, Space, Tag } from '@douyinfe/semi-ui';

import CardTable from '../common/ui/CardTable';
import MonitorTimeline from './MonitorTimeline';
import {
  formatMonitorAvailability,
  formatMonitorLatency,
  getMonitorStatusMeta,
} from './monitorTimelineUtils';

const MonitorStatusTable = ({ groups, loading, onOpen, t }) => {
  const columns = [
    {
      title: t('调用分组'),
      dataIndex: 'name',
      key: 'name',
      render: (_, row) => (
        <Button theme='borderless' size='small' onClick={() => onOpen(row)}>
          {row.name}
        </Button>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      render: (value) => {
        const status = getMonitorStatusMeta(value);
        return (
          <Tag color={status.color} shape='circle'>
            {t(status.label)}
          </Tag>
        );
      },
    },
    {
      title: t('渠道类型'),
      dataIndex: 'channel_types',
      key: 'channel_types',
      render: (types = []) => (
        <Space spacing={4} wrap>
          {types.map((type) => (
            <Tag key={type} color='blue' size='small'>
              {type}
            </Tag>
          ))}
        </Space>
      ),
    },
    {
      title: t('主探测模型'),
      dataIndex: 'primary_model',
      key: 'primary_model',
    },
    {
      title: t('模型延迟'),
      dataIndex: 'current_latency_ms',
      key: 'latency',
      render: formatMonitorLatency,
    },
    {
      title: t('端点 Ping'),
      dataIndex: 'current_ping_latency_ms',
      key: 'ping',
      render: formatMonitorLatency,
    },
    {
      title: t('可用率'),
      dataIndex: 'availability',
      key: 'availability',
      render: (_, row) => formatMonitorAvailability(row),
    },
    {
      title: t('近 60 次记录'),
      dataIndex: 'timeline',
      key: 'timeline',
      render: (timeline) => (
        <div className='w-48'>
          <MonitorTimeline timeline={timeline} />
        </div>
      ),
    },
  ];

  return (
    <CardTable
      columns={columns}
      dataSource={groups}
      rowKey='id'
      pagination={false}
      loading={loading}
      scroll={{ x: 'max-content' }}
      onRow={(row) => ({
        onClick: () => onOpen(row),
        className: 'cursor-pointer',
      })}
      empty={t('暂无可展示的监控分组')}
      className='overflow-hidden rounded-xl'
    />
  );
};

export default MonitorStatusTable;
