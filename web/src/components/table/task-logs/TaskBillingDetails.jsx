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
import { Descriptions } from '@douyinfe/semi-ui';
import { renderQuota, timestamp2string } from '../../../helpers';
import {
  taskBillingSummary,
  taskDiagnosticFields,
} from '../../../helpers/h3Task';

export const settlementLabel = (status, t) =>
  ({
    pending: t('待结算'),
    applying: t('结算处理中'),
    applied: t('已结算'),
    failed: t('结算待处理'),
    legacy: t('历史账务'),
  })[status] || t('暂无账务数据');

const TaskBillingDetails = ({ task, admin = false, root = false, t }) => {
  const summary = taskBillingSummary(task);
  const amount = (value) => (value === null ? '-' : renderQuota(value, 6));
  const rows = [
    { key: t('账务状态'), value: settlementLabel(task.settlement_status, t) },
    { key: t('预扣金额'), value: amount(summary.precharge) },
    { key: t('最终扣费'), value: amount(summary.final) },
  ];
  if (summary.refund !== null)
    rows.push({ key: t('已退还金额'), value: amount(summary.refund) });
  if (summary.additional !== null)
    rows.push({ key: t('补扣金额'), value: amount(summary.additional) });
  if (task.settled_at > 0)
    rows.push({ key: t('结算时间'), value: timestamp2string(task.settled_at) });
  const labels = {
    poll_failures: t('连续轮询失败次数'),
    last_poll_error_class: t('轮询错误分类'),
    last_poll_error_message: t('脱敏上游摘要'),
    last_poll_at: t('最近轮询时间'),
    next_poll_at: t('下次轮询时间'),
    settlement_id: t('结算记录 ID'),
    pricing_version: t('价格版本'),
  };
  for (const [key, value] of Object.entries(
    taskDiagnosticFields(task, { admin, root }),
  )) {
    rows.push({
      key: labels[key],
      value: ['last_poll_at', 'next_poll_at'].includes(key)
        ? value > 0
          ? timestamp2string(value)
          : '-'
        : typeof value === 'string' || typeof value === 'number'
          ? String(value)
          : '-',
    });
  }
  return (
    <Descriptions data={rows} column={1} style={{ overflowWrap: 'anywhere' }} />
  );
};
export default TaskBillingDetails;
