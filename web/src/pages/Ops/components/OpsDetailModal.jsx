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
import { Modal, Pagination, Table, Tag, Typography } from '@douyinfe/semi-ui';

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

const formatMilliseconds = (value) => {
  if (value === null || value === undefined || value === '') return '--';
  const number = Number(value);
  return Number.isFinite(number) ? `${number.toLocaleString()} ms` : '--';
};

const formatTokens = (row) => {
  const input = Number(row.prompt_tokens || 0).toLocaleString();
  const output = Number(row.completion_tokens || 0).toLocaleString();
  return `${input} / ${output}`;
};

const errorColumns = (t) => [
  {
    title: t('错误分类'),
    dataIndex: 'error_class',
    width: 125,
    render: (value) => value || '--',
  },
  {
    title: t('错误码'),
    dataIndex: 'error_code',
    width: 155,
    render: (value, row) => value || row.status_code || '--',
  },
  {
    title: t('错误类型'),
    dataIndex: 'error_type',
    width: 145,
    render: (value) => value || '--',
  },
  {
    title: t('错误信息'),
    dataIndex: 'error_message',
    width: 260,
    render: (value, row) => (
      <Typography.Paragraph
        ellipsis={{ rows: 2, showTooltip: true }}
        style={{ marginBottom: 0, maxWidth: 250 }}
      >
        {value || row.error_type || '--'}
      </Typography.Paragraph>
    ),
  },
];

const buildRequestColumns = (metric, t) => {
  const columns = [
    {
      title: t('时间'),
      dataIndex: 'created_at',
      width: 175,
      render: formatOpsTimestamp,
    },
    { title: t('模型'), dataIndex: 'model_name', width: 155 },
    { title: t('调用分组'), dataIndex: 'group', width: 120 },
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      width: 150,
      render: (value, row) => value || `#${row.channel_id}`,
    },
    {
      title: t('状态码'),
      dataIndex: 'status_code',
      width: 85,
      render: (value, row) => (
        <Tag color={row.type === 5 ? 'red' : 'green'} size='small'>
          {value || (row.type === 5 ? t('失败') : t('成功'))}
        </Tag>
      ),
    },
    {
      title: t('总耗时'),
      dataIndex: 'total_latency_ms',
      width: 105,
      render: formatMilliseconds,
    },
    {
      title: 'TTFT',
      dataIndex: 'ttft_ms',
      width: 95,
      render: formatMilliseconds,
    },
    {
      title: t('Token 数'),
      key: 'tokens',
      width: 125,
      render: (_, row) => formatTokens(row),
    },
    {
      title: t('额度'),
      dataIndex: 'quota',
      width: 105,
      render: (value) => Number(value || 0).toLocaleString(),
    },
    {
      title: t('请求 ID'),
      dataIndex: 'request_id',
      width: 195,
      render: (value) => (
        <Typography.Text ellipsis={{ showTooltip: true }}>
          {value || '--'}
        </Typography.Text>
      ),
    },
  ];
  if (metric === 'errors' || metric === 'upstream') {
    columns.splice(5, 0, ...errorColumns(t));
  }
  return columns;
};

const redactEventValue = (value, key = '') => {
  const sensitive =
    /(^|_)(api_?key|key|token|password|secret|authorization|credential)(_|$)/i;
  if (sensitive.test(key)) return '[REDACTED]';
  if (Array.isArray(value)) return value.map((item) => redactEventValue(item));
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value).map(([childKey, childValue]) => [
        childKey,
        redactEventValue(childValue, childKey),
      ]),
    );
  }
  return value;
};

const parseEventExtra = (extra) => {
  if (!extra) return null;
  try {
    return redactEventValue(JSON.parse(extra));
  } catch {
    return extra;
  }
};

const EventDetailContent = ({ event, openRequestDetail, t }) => {
  if (!event) return null;
  const extra = parseEventExtra(event.extra);
  const rows = [
    [t('事件 ID'), event.id],
    [t('时间'), formatOpsTimestamp(event.created_at)],
    [t('级别'), event.level],
    [t('组件'), event.component],
    [t('请求 ID'), event.request_id || '--'],
    [t('渠道'), event.channel_id ? `#${event.channel_id}` : '--'],
    [t('模型'), event.model_name || '--'],
    [t('调用分组'), event.group || '--'],
    [t('状态码'), event.status_code || '--'],
    [t('总耗时'), formatMilliseconds(event.latency_ms)],
  ];
  return (
    <>
      <div className='grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3'>
        {rows.map(([label, value]) => (
          <div key={label} className='min-w-0'>
            <Typography.Text type='tertiary' size='small'>
              {label}
            </Typography.Text>
            <div className='mt-1 break-words text-sm'>{value}</div>
          </div>
        ))}
      </div>
      <div className='mt-5'>
        <Typography.Text type='tertiary' size='small'>
          {t('错误信息')}
        </Typography.Text>
        <div className='mt-1 whitespace-pre-wrap break-words rounded-md border p-3 text-sm'>
          {event.message || t('暂无详细信息')}
        </div>
      </div>
      {extra && (
        <div className='mt-4'>
          <Typography.Text type='tertiary' size='small'>
            {t('额外信息')}
          </Typography.Text>
          <pre className='mt-1 max-h-64 overflow-auto rounded-md border p-3 text-xs'>
            {typeof extra === 'string' ? extra : JSON.stringify(extra, null, 2)}
          </pre>
        </div>
      )}
      {event.request_id && (
        <div className='mt-5 flex justify-end'>
          <button
            type='button'
            className='text-sm text-blue-600 hover:underline'
            onClick={() => openRequestDetail?.(event.request_id)}
          >
            {t('查看请求详情')}
          </button>
        </div>
      )}
    </>
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
  eventDetail,
  openDetail,
  t,
}) => {
  const showingEvent = Boolean(eventDetail);
  const columns = buildRequestColumns(detailMetric?.key, t);

  return (
    <Modal
      visible={showingEvent || !!detailMetric}
      title={
        showingEvent
          ? t('事件详情')
          : `${detailMetric?.title || ''}${t('明细')}`
      }
      width={showingEvent ? 760 : 1180}
      footer={null}
      onCancel={closeDetail}
    >
      {showingEvent ? (
        <EventDetailContent
          event={eventDetail}
          openRequestDetail={(requestId) =>
            openDetail({ key: 'requests', title: t('请求详情') }, requestId)
          }
          t={t}
        />
      ) : (
        <>
          <OpsDetailSummary metric={detailMetric} overview={overview} t={t} />
          <Table
            columns={columns}
            dataSource={detailData.items}
            rowKey={(row) => row.id || row.request_id}
            loading={detailsLoading}
            pagination={false}
            scroll={{ x: 'max-content' }}
            empty={t('暂无请求数据')}
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
        </>
      )}
    </Modal>
  );
};

export default OpsDetailModal;
