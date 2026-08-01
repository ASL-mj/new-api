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
import { Modal, Spin, Tag, Typography } from '@douyinfe/semi-ui';

import MonitorTimeline from './MonitorTimeline';
import {
  formatMonitorAvailability,
  formatMonitorLatency,
  getMonitorStatusMeta,
} from './monitorTimelineUtils';

const MonitorStatusDetailModal = ({
  selected,
  detailLoading,
  closeDetails,
  t,
}) => {
  const status = getMonitorStatusMeta(selected?.status);
  return (
    <Modal
      title={selected?.name || t('分组详情')}
      visible={!!selected}
      onCancel={closeDetails}
      footer={null}
      width={680}
    >
      {selected && (
        <Spin spinning={detailLoading}>
          <div className='space-y-5'>
            <div className='flex items-center justify-between gap-3'>
              <div>
                <Typography.Text type='tertiary'>
                  {t('主探测模型')}
                </Typography.Text>
                <div className='mt-1 font-medium'>
                  {selected.primary_model || '--'}
                </div>
              </div>
              <Tag color={status.color} shape='circle'>
                {t(status.label)}
              </Tag>
            </div>
            <div className='grid grid-cols-2 gap-4 sm:grid-cols-4'>
              <div>
                <Typography.Text type='tertiary'>
                  {t('模型延迟')}
                </Typography.Text>
                <div className='mt-1 font-semibold'>
                  {formatMonitorLatency(selected.current_latency_ms)}
                </div>
              </div>
              <div>
                <Typography.Text type='tertiary'>
                  {t('端点 Ping')}
                </Typography.Text>
                <div className='mt-1 font-semibold'>
                  {formatMonitorLatency(selected.current_ping_latency_ms)}
                </div>
              </div>
              <div>
                <Typography.Text type='tertiary'>{t('可用率')}</Typography.Text>
                <div className='mt-1 font-semibold'>
                  {formatMonitorAvailability(selected)}
                </div>
              </div>
              <div>
                <Typography.Text type='tertiary'>
                  {t('主探测模型真实请求成功率')}
                </Typography.Text>
                <div className='mt-1 font-semibold'>
                  {selected.real_success_rate == null
                    ? '--'
                    : `${Number(selected.real_success_rate).toFixed(2)}%`}
                </div>
              </div>
            </div>
            <div>
              <div className='mb-2 text-sm font-medium'>
                {t('近 60 次记录')}
              </div>
              <MonitorTimeline timeline={selected.timeline} t={t} />
            </div>
            <div>
              <div className='mb-2 text-sm font-medium'>{t('每日可用率')}</div>
              {selected.availability_history?.length ? (
                <div className='max-h-64 divide-y overflow-y-auto'>
                  {selected.availability_history.map((item) => (
                    <div
                      key={item.bucket_ts}
                      className='flex items-center justify-between gap-3 py-2 text-sm'
                    >
                      <span>
                        {new Date(item.bucket_ts * 1000).toLocaleDateString()}
                      </span>
                      <span>
                        {Number(item.availability_pct).toFixed(2)}% ·{' '}
                        {item.available_count}/{item.check_count}
                      </span>
                    </div>
                  ))}
                </div>
              ) : (
                <Typography.Text type='tertiary'>
                  {t('所选周期内暂无探测历史')}
                </Typography.Text>
              )}
            </div>
          </div>
        </Spin>
      )}
    </Modal>
  );
};

export default MonitorStatusDetailModal;
