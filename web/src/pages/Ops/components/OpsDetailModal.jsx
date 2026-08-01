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
import { Modal, Pagination, Table, Typography } from '@douyinfe/semi-ui';

import { formatOpsTimestamp } from '../../../hooks/ops/useOpsData';
import { buildOpsMetricItems } from './OpsMetricGrid';

const OpsDetailSummary = ({ metric, overview, t }) => {
  if (!metric) return null;
  const item = buildOpsMetricItems(overview, t).find(
    (candidate) => candidate.key === metric.key,
  );
  if (!item) return null;
  const summaryRows = [
    ...(item.value !== undefined
      ? [
          {
            label: item.title,
            value: `${item.value}${item.unit ? ` ${item.unit}` : ''}`,
          },
        ]
      : []),
    ...item.rows,
  ];
  return (
    <div className='mb-4 grid grid-cols-2 gap-2 sm:grid-cols-4'>
      {summaryRows.map((row) => (
        <div
          key={row.label}
          className='rounded-lg border p-3'
          style={{
            background: 'var(--semi-color-fill-0)',
            borderColor: 'var(--semi-color-border)',
          }}
        >
          <Typography.Text type='tertiary' size='small'>
            {row.label}
          </Typography.Text>
          <div className='mt-1 truncate text-base font-semibold tabular-nums'>
            {row.value}
          </div>
        </div>
      ))}
    </div>
  );
};

const OpsDetailModal = ({
  detailMetric,
  detailData,
  detailPage,
  detailsLoading,
  closeDetail,
  handleDetailPageChange,
  overview,
  t,
}) => {
  const baseColumns = [
    {
      title: t('时间'),
      dataIndex: 'bucket_ts',
      width: 175,
      render: formatOpsTimestamp,
    },
    { title: t('模型'), dataIndex: 'model_name', width: 160 },
    { title: t('调用分组'), dataIndex: 'group', width: 120 },
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      width: 150,
      render: (value, row) => value || `#${row.channel_id}`,
    },
  ];
  const metricColumns = {
    requests: [
      { title: t('请求'), dataIndex: 'request_count', width: 75 },
      { title: t('成功'), dataIndex: 'success_count', width: 75 },
    ],
    sla: [
      { title: t('请求'), dataIndex: 'request_count', width: 75 },
      { title: t('成功'), dataIndex: 'success_count', width: 75 },
      {
        title: 'SLA',
        dataIndex: 'sla',
        width: 85,
        render: (value) => `${Number(value || 0).toFixed(2)}%`,
      },
    ],
    errors: [
      { title: t('请求'), dataIndex: 'request_count', width: 75 },
      {
        title: t('错误率'),
        dataIndex: 'error_rate',
        width: 90,
        render: (value) => `${Number(value || 0).toFixed(2)}%`,
      },
    ],
    upstream: [
      { title: t('请求'), dataIndex: 'request_count', width: 75 },
      { title: t('上游错误'), dataIndex: 'upstream_errors', width: 95 },
    ],
    ttft: [
      { title: t('请求'), dataIndex: 'request_count', width: 75 },
      {
        title: t('平均 TTFT'),
        dataIndex: 'avg_ttft_ms',
        width: 105,
        render: (value) => `${value || 0} ms`,
      },
    ],
    duration: [
      { title: t('请求'), dataIndex: 'request_count', width: 75 },
      {
        title: t('平均时长'),
        dataIndex: 'avg_duration_ms',
        width: 105,
        render: (value) => `${value || 0} ms`,
      },
    ],
  };
  const columns = [
    ...baseColumns,
    ...(metricColumns[detailMetric?.key] || metricColumns.requests),
  ];

  return (
    <Modal
      visible={!!detailMetric}
      title={`${detailMetric?.title || ''}${t('明细')}`}
      width={1180}
      footer={null}
      onCancel={closeDetail}
    >
      <OpsDetailSummary metric={detailMetric} overview={overview} t={t} />
      <Table
        columns={columns}
        dataSource={detailData.items}
        rowKey={(row) =>
          `${row.bucket_ts}-${row.channel_id}-${row.group}-${row.model_name}`
        }
        loading={detailsLoading}
        pagination={false}
        scroll={{ x: 'max-content' }}
        empty={t('所选条件下暂无聚合记录')}
        size='small'
      />
      {detailData.total > 20 && (
        <div className='mt-3 flex justify-end'>
          <Pagination
            size='small'
            currentPage={detailPage}
            pageSize={20}
            total={detailData.total}
            onPageChange={handleDetailPageChange}
          />
        </div>
      )}
    </Modal>
  );
};

export default OpsDetailModal;
