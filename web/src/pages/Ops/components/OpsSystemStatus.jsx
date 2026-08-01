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
import { Banner, Card, Progress, Spin, Typography } from '@douyinfe/semi-ui';
import {
  Cpu,
  Database,
  ListChecks,
  MemoryStick,
  ScrollText,
  Workflow,
} from 'lucide-react';

const percentColor = (value) => {
  if (value >= 90) return 'var(--semi-color-danger)';
  if (value >= 75) return 'var(--semi-color-warning)';
  return 'var(--semi-color-success)';
};

const StatusBlock = ({ icon, label, value, detail, progress }) => (
  <div
    className='min-w-0 border-b p-3 last:border-b-0 sm:[&:nth-last-child(-n+2)]:border-b-0 xl:border-b-0 xl:border-r xl:last:border-r-0'
    style={{ borderColor: 'var(--semi-color-border)' }}
  >
    <div
      className='flex items-center gap-2'
      style={{ color: 'var(--semi-color-text-2)' }}
    >
      {icon}
      <Typography.Text type='tertiary'>{label}</Typography.Text>
    </div>
    <div className='mt-2 truncate text-xl font-semibold'>{value}</div>
    {progress != null && (
      <Progress
        percent={Math.min(100, Math.max(0, progress))}
        showInfo={false}
        stroke={percentColor(progress)}
        size='small'
        className='mt-2'
      />
    )}
    <div
      className='mt-1 truncate text-xs'
      style={{ color: 'var(--semi-color-text-2)' }}
      title={detail}
    >
      {detail}
    </div>
  </div>
);

const OpsSystemStatus = ({ system, systemLoading, systemError, t }) => {
  const tasks = system?.background_tasks || {};
  const writer = system?.system_event_writer || {};
  const blocks = [
    {
      key: 'cpu',
      icon: <Cpu size={16} />,
      label: t('CPU'),
      value: system ? `${Number(system.cpu_usage || 0).toFixed(1)}%` : '--',
      detail: t('当前进程采样'),
      progress: system?.cpu_usage,
    },
    {
      key: 'memory',
      icon: <MemoryStick size={16} />,
      label: t('内存'),
      value: system ? `${Number(system.memory_usage || 0).toFixed(1)}%` : '--',
      detail: t('系统内存使用率'),
      progress: system?.memory_usage,
    },
    {
      key: 'database',
      icon: <Database size={16} />,
      label: t('数据库'),
      value: system
        ? `${system.in_use || 0} / ${system.open_connections || 0}`
        : '--',
      detail: `${t('空闲')} ${system?.idle || 0} · ${t('等待')} ${system?.wait_count || 0}`,
    },
    {
      key: 'goroutines',
      icon: <Workflow size={16} />,
      label: t('Go 协程'),
      value: system?.goroutines ?? '--',
      detail: t('当前运行协程数'),
    },
    {
      key: 'tasks',
      icon: <ListChecks size={16} />,
      label: t('后台任务'),
      value:
        tasks.total == null ? '--' : `${tasks.healthy || 0} / ${tasks.total}`,
      detail: `${t('异常')} ${tasks.error || 0} · ${t('过期')} ${tasks.stale || 0}`,
    },
    {
      key: 'queue',
      icon: <ScrollText size={16} />,
      label: t('日志队列'),
      value:
        writer.capacity == null
          ? '--'
          : `${writer.pending_count || 0} / ${writer.capacity}`,
      detail: `${t('写入')} ${writer.written_count || 0} · ${t('丢弃')} ${writer.dropped_count || 0} · ${t('失败')} ${writer.write_failed_count || 0}`,
    },
  ];

  return (
    <Card
      className='mb-3 !rounded-lg'
      title={t('服务器状态')}
      bodyStyle={{ padding: 0 }}
    >
      {systemError && (
        <Banner type='danger' description={systemError} className='m-3' />
      )}
      {systemLoading && !system ? (
        <div className='flex h-36 items-center justify-center'>
          <Spin />
        </div>
      ) : (
        <div className='grid grid-cols-2 xl:grid-cols-6'>
          {blocks.map((block) => (
            <StatusBlock key={block.key} {...block} />
          ))}
        </div>
      )}
    </Card>
  );
};

export default OpsSystemStatus;
