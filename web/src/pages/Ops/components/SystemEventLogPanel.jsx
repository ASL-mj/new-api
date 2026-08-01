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
  Banner,
  Button,
  Card,
  Input,
  Pagination,
  Select,
  Tag,
} from '@douyinfe/semi-ui';
import { RotateCcw, Search } from 'lucide-react';

import CardTable from '../../../components/common/ui/CardTable';
import { formatOpsTimestamp } from '../../../hooks/ops/useOpsData';

const SystemEventLogPanel = ({
  logDraft,
  setLogDraft,
  logs,
  logPage,
  logsLoading,
  logsError,
  applyLogFilters,
  resetLogFilters,
  handleLogPageChange,
  t,
}) => {
  const columns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      key: 'created_at',
      width: 175,
      render: formatOpsTimestamp,
    },
    {
      title: t('级别'),
      dataIndex: 'level',
      key: 'level',
      width: 80,
      render: (value) => (
        <Tag
          color={
            value === 'error' ? 'red' : value === 'warn' ? 'orange' : 'blue'
          }
          size='small'
        >
          {value}
        </Tag>
      ),
    },
    { title: t('组件'), dataIndex: 'component', key: 'component', width: 150 },
    { title: t('事件'), dataIndex: 'message', key: 'message' },
    {
      title: t('请求 ID'),
      dataIndex: 'request_id',
      key: 'request_id',
      width: 190,
      render: (value) => value || '--',
    },
    {
      title: t('状态码'),
      dataIndex: 'status_code',
      key: 'status_code',
      width: 80,
      render: (value) => value || '--',
    },
  ];

  return (
    <Card
      className='mt-3 !rounded-lg'
      title={t('系统运行日志')}
      bodyStyle={{ padding: 12 }}
    >
      <div className='mb-3 flex flex-col gap-2 lg:flex-row lg:justify-end'>
        <Select
          size='small'
          value={logDraft.level}
          placeholder={t('全部级别')}
          style={{ width: 130 }}
          optionList={[
            { label: t('全部级别'), value: '' },
            { label: t('信息'), value: 'info' },
            { label: t('警告'), value: 'warn' },
            { label: t('错误'), value: 'error' },
          ]}
          onChange={(value) =>
            setLogDraft((current) => ({ ...current, level: value || '' }))
          }
        />
        <Input
          size='small'
          value={logDraft.component}
          placeholder={t('组件名称')}
          style={{ width: 160 }}
          showClear
          onChange={(value) =>
            setLogDraft((current) => ({ ...current, component: value }))
          }
        />
        <Input
          size='small'
          value={logDraft.requestId}
          placeholder={t('请求 ID')}
          style={{ width: 210 }}
          showClear
          onChange={(value) =>
            setLogDraft((current) => ({ ...current, requestId: value }))
          }
        />
        <Button
          size='small'
          type='primary'
          icon={<Search size={14} />}
          onClick={applyLogFilters}
        >
          {t('查询')}
        </Button>
        <Button
          size='small'
          type='tertiary'
          icon={<RotateCcw size={14} />}
          onClick={resetLogFilters}
        >
          {t('重置')}
        </Button>
      </div>
      {logsError && (
        <Banner type='danger' description={logsError} className='mb-3' />
      )}
      <CardTable
        columns={columns}
        dataSource={logs.items}
        rowKey='id'
        loading={logsLoading}
        pagination={false}
        scroll={{ x: 'max-content' }}
        empty={t('暂无系统事件')}
        size='small'
      />
      {logs.total > 10 && (
        <div className='mt-3 flex justify-end'>
          <Pagination
            size='small'
            currentPage={logPage}
            pageSize={10}
            total={logs.total}
            onPageChange={handleLogPageChange}
          />
        </div>
      )}
    </Card>
  );
};

export default SystemEventLogPanel;
