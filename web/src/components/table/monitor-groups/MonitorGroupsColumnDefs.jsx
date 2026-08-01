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
import {
  Button,
  Popconfirm,
  Space,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  Pencil,
  Power,
  PowerOff,
  RefreshCw,
  Trash2,
} from 'lucide-react';

import { timestamp2string } from '../../../helpers';

const renderTypes = (types, t) => {
  if (!types?.length) return <Tag color='grey'>{t('未知')}</Tag>;
  return (
    <Space spacing={4} wrap>
      {types.map((type) => (
        <Tag key={type} color='blue' shape='circle' size='small'>
          {type}
        </Tag>
      ))}
    </Space>
  );
};

export const getMonitorGroupsColumns = ({
  runGroup,
  updateEnabled,
  openEdit,
  deleteGroup,
  saving,
  t,
}) => [
  {
    title: t('监控分组'),
    dataIndex: 'name',
    key: 'name',
    render: (_, row) => (
      <div className='min-w-44'>
        <Typography.Text strong>{row.name}</Typography.Text>
        <div>
          <Typography.Text type='tertiary' size='small'>
            {row.key}
            {row.description ? ` · ${row.description}` : ''}
          </Typography.Text>
        </div>
      </div>
    ),
  },
  {
    title: t('运行状态'),
    dataIndex: 'enabled',
    key: 'status',
    render: (_, row) => {
      if (row.running) {
        return (
          <Tag color='blue' shape='circle' prefixIcon={<Activity size={12} />}>
            {t('探测中')}
          </Tag>
        );
      }
      return (
        <Tag color={row.enabled ? 'green' : 'grey'} shape='circle'>
          {row.enabled ? t('已启用') : t('已停用')}
        </Tag>
      );
    },
  },
  {
    title: t('渠道类型'),
    dataIndex: 'channel_types',
    key: 'channel_types',
    render: (types) => renderTypes(types, t),
  },
  {
    title: t('主探测模型'),
    dataIndex: 'primary_model',
    key: 'primary_model',
  },
  {
    title: t('渠道数'),
    dataIndex: 'targets',
    key: 'target_count',
    render: (targets) => targets?.length || 0,
  },
  {
    title: t('探测间隔'),
    dataIndex: 'interval_seconds',
    key: 'interval',
    render: (seconds) => `${seconds} s`,
  },
  {
    title: t('用户可见'),
    dataIndex: 'user_visible',
    key: 'user_visible',
    render: (visible) => (
      <Tag color={visible ? 'green' : 'grey'} shape='circle' size='small'>
        {visible ? t('是') : t('否')}
      </Tag>
    ),
  },
  {
    title: t('最近探测'),
    dataIndex: 'last_checked_at',
    key: 'last_checked_at',
    render: (timestamp) =>
      timestamp > 0 ? timestamp2string(timestamp) : t('尚未探测'),
  },
  {
    title: '',
    dataIndex: 'operate',
    key: 'operate',
    fixed: 'right',
    render: (_, row) => (
      <Space spacing={2}>
        <Tooltip content={t('立即探测')}>
          <Button
            theme='borderless'
            type='tertiary'
            size='small'
            icon={
              <RefreshCw
                size={15}
                className={row.running ? 'animate-spin' : ''}
              />
            }
            loading={false}
            disabled={row.running}
            aria-label={t('立即探测')}
            onClick={() => runGroup(row)}
          />
        </Tooltip>
        <Tooltip content={row.enabled ? t('停用') : t('启用')}>
          <Button
            theme='borderless'
            type={row.enabled ? 'warning' : 'tertiary'}
            size='small'
            icon={row.enabled ? <PowerOff size={15} /> : <Power size={15} />}
            loading={saving}
            aria-label={row.enabled ? t('停用') : t('启用')}
            onClick={() => updateEnabled(row, !row.enabled)}
          />
        </Tooltip>
        <Tooltip content={t('编辑')}>
          <Button
            theme='borderless'
            type='tertiary'
            size='small'
            icon={<Pencil size={15} />}
            aria-label={t('编辑')}
            onClick={() => openEdit(row)}
          />
        </Tooltip>
        <Popconfirm
          title={t('删除监控分组')}
          content={t('分组配置会被删除，历史探测记录将保留。')}
          onConfirm={() => deleteGroup(row)}
        >
          <Tooltip content={t('删除')}>
            <Button
              theme='borderless'
              type='danger'
              size='small'
              icon={<Trash2 size={15} />}
              aria-label={t('删除')}
            />
          </Tooltip>
        </Popconfirm>
      </Space>
    ),
  },
];
