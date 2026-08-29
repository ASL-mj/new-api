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
        if (key === 'key_quota_unlimited') {
          total[key] = total[key] || Number(current[key] || false);
          return;
        }
        if (key === 'quota_limit_mode') {
          if (total[key] === undefined) {
            total[key] = current[key];
          } else if (total[key] !== current[key]) {
            total[key] = null;
          }
          return;
        }
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
      quota_limit_mode: undefined,
      key_quota_limit_used: 0,
      key_quota_limit: 0,
      key_quota_unlimited: false,
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

// 多密钥独立限额的汇总展示：分子分母同为密钥口径；
// 存在未配置限额的启用密钥时，合计上限视为无限。
const buildKeyLimitText = (value) => {
  const used = Number(value.key_quota_limit_used || 0);
  if (value.key_quota_unlimited) {
    return { used, limitText: '∞' };
  }
  return { used, limitText: renderQuota(Number(value.key_quota_limit || 0)) };
};

export const UsedAndLimitCell = ({ record, stats, loading, error, t }) => {
  const value = aggregateStats(record, stats) || {};
  const mode = value.quota_limit_mode;
  const limit = Number(value.quota_limit || 0);
  const channelUsed = renderQuota(value.quota_limit_used || 0);
  const channelLimitText = limit > 0 ? renderQuota(limit) : '∞';

  if (mode === 'key') {
    const { used, limitText } = buildKeyLimitText(value);
    return renderState(loading, error, lines(renderQuota(used), limitText));
  }

  if (mode === 'both') {
    const { used, limitText } = buildKeyLimitText(value);
    return renderState(
      loading,
      error,
      <Tooltip
        content={`${t('密钥限额合计')}：${renderQuota(used)} / ${limitText}`}
      >
        <span>{lines(channelUsed, channelLimitText)}</span>
      </Tooltip>,
    );
  }

  return renderState(loading, error, lines(channelUsed, channelLimitText));
};

export const UpstreamBalanceCell = ({
  record,
  stats,
  loading,
  error,
  onRefresh,
  t,
}) => {
  const value = aggregateStats(record, stats) || {};
  return renderState(
    loading,
    error,
    <Tooltip content={record.children ? undefined : t('点击更新上游余额')}>
      <span
        className={record.children ? '' : 'cursor-pointer'}
        onClick={() => !record.children && onRefresh(record)}
      >
        {renderQuotaWithAmount(value.balance || 0)}
      </span>
    </Tooltip>,
  );
};
