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
  monitorTimelineHeight,
  padMonitorTimeline,
} from './monitorTimelineUtils';

const pointColor = (status) => {
  switch (status) {
    case 'operational':
      return 'var(--semi-color-success)';
    case 'degraded':
      return 'var(--semi-color-warning)';
    case 'failed':
    case 'timeout':
      return 'var(--semi-color-danger)';
    default:
      return 'var(--semi-color-fill-1)';
  }
};

const MonitorTimeline = ({ timeline }) => (
  <div>
    <div className='flex h-5 items-end gap-px' aria-label='近 60 次探测记录'>
      {padMonitorTimeline(timeline).map((point, index) => (
        <span
          key={`${point.checked_at ?? 'empty'}-${index}`}
          className='min-w-0 flex-1 rounded-sm'
          style={{
            height: `${monitorTimelineHeight(point.status)}%`,
            backgroundColor: pointColor(point.status),
          }}
          title={point.status === 'empty' ? '暂无记录' : point.status}
        />
      ))}
    </div>
    <div
      className='mt-1 flex justify-between text-xs'
      style={{ color: 'var(--semi-color-text-2)' }}
    >
      <span>past</span>
      <span>now</span>
    </div>
  </div>
);

export default MonitorTimeline;
