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
import { Card, Empty, Tag, Typography } from '@douyinfe/semi-ui';

import { formatOpsTimestamp } from '../../../hooks/ops/useOpsData';

const OpsAlerts = ({ alerts = [], openEventDetail, t }) => (
  <Card
    className='h-full !rounded-lg'
    title={t('最新告警')}
    bodyStyle={{ padding: 12 }}
  >
    {alerts.length === 0 ? (
      <Empty description={t('暂无警告或错误事件')} />
    ) : (
      <div className='max-h-72 divide-y overflow-y-auto'>
        {alerts.map((alert, index) => (
          <button
            type='button'
            key={`${alert.created_at}-${alert.component}-${index}`}
            className='block w-full border-0 bg-transparent py-3 text-left first:pt-0 last:pb-0'
            style={{ borderColor: 'var(--semi-color-border)' }}
            onClick={() => openEventDetail?.(alert)}
          >
            <div className='flex items-center justify-between gap-2'>
              <Tag
                color={alert.level === 'error' ? 'red' : 'orange'}
                size='small'
              >
                {alert.level}
              </Tag>
              <Typography.Text type='tertiary' size='small'>
                {formatOpsTimestamp(alert.created_at)}
              </Typography.Text>
            </div>
            <div className='mt-2 text-sm font-medium'>{alert.component}</div>
            <Typography.Paragraph
              type='tertiary'
              ellipsis={{ rows: 2, showTooltip: true }}
              style={{ marginBottom: 0 }}
            >
              {alert.message}
            </Typography.Paragraph>
          </button>
        ))}
      </div>
    )}
  </Card>
);

export default OpsAlerts;
