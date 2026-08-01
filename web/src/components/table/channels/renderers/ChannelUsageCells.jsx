import React from 'react';
import { Tooltip, Typography } from '@douyinfe/semi-ui';
import { renderQuota, renderQuotaWithAmount } from '../../../../helpers';

const { Text } = Typography;

const lines = (top, bottom) => (
  <div className='flex flex-col leading-5 whitespace-nowrap'>
    <Text size='small'>{top}</Text>
    <Text size='small' type='tertiary'>
      {bottom}
    </Text>
  </div>
);

const aggregateStats = (record, stats) => {
  if (!Array.isArray(record.children)) return stats[record.id];
  return record.children.reduce(
    (total, child) => {
      const current = stats[child.id] || {};
      Object.keys(total).forEach((key) => {
        total[key] += Number(current[key] || 0);
      });
      return total;
    },
    {
      today_quota: 0,
      last30d_quota: 0,
      quota_limit_used: 0,
      quota_limit: 0,
      balance: 0,
    },
  );
};

const renderState = (loading, error, content) => {
  if (loading) return <Text type='tertiary'>...</Text>;
  if (error) return <Tooltip content={error}>-</Tooltip>;
  return content;
};

export const TodayAnd30dUsageCell = ({ record, stats, loading, error }) => {
  const value = aggregateStats(record, stats) || {};
  return renderState(
    loading,
    error,
    lines(
      renderQuota(value.today_quota || 0),
      renderQuota(value.last30d_quota || 0),
    ),
  );
};

export const UsedAndLimitCell = ({ record, stats, loading, error }) => {
  const value = aggregateStats(record, stats) || {};
  const limit = Number(value.quota_limit || 0);
  return renderState(
    loading,
    error,
    lines(
      renderQuota(value.quota_limit_used || 0),
      limit > 0 ? renderQuota(limit) : '∞',
    ),
  );
};

export const UpstreamBalanceCell = ({
  record,
  stats,
  loading,
  error,
  onRefresh,
}) => {
  const value = aggregateStats(record, stats) || {};
  return renderState(
    loading,
    error,
    <Tooltip content={record.children ? undefined : '点击更新上游余额'}>
      <span
        className={record.children ? '' : 'cursor-pointer'}
        onClick={() => !record.children && onRefresh(record)}
      >
        {renderQuotaWithAmount(value.balance || 0)}
      </span>
    </Tooltip>,
  );
};
