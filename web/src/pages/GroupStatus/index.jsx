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
import { Empty, Spin } from '@douyinfe/semi-ui';

import MonitorStatusCard from '../../components/monitor-status/MonitorStatusCard';
import MonitorStatusDetailModal from '../../components/monitor-status/MonitorStatusDetailModal';
import MonitorStatusTable from '../../components/monitor-status/MonitorStatusTable';
import MonitorStatusToolbar from '../../components/monitor-status/MonitorStatusToolbar';
import { useMonitorStatusData } from '../../hooks/monitor-status/useMonitorStatusData';

const GroupStatus = () => {
  const data = useMonitorStatusData();
  return (
    <div className='mt-[60px] px-2'>
      <MonitorStatusToolbar {...data} />
      {data.loading ? (
        <div className='flex justify-center py-16'>
          <Spin />
        </div>
      ) : data.groups.length === 0 ? (
        <Empty description={data.t('暂无可展示的监控分组')} />
      ) : data.view === 'card' ? (
        <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5'>
          {data.groups.map((group) => (
            <MonitorStatusCard
              key={group.id}
              group={group}
              onOpen={data.openDetails}
              t={data.t}
              refreshAfter={data.refreshAfter}
            />
          ))}
        </div>
      ) : (
        <MonitorStatusTable
          groups={data.groups}
          loading={data.loading}
          onOpen={data.openDetails}
          t={data.t}
        />
      )}
      <MonitorStatusDetailModal {...data} />
    </div>
  );
};

export default GroupStatus;
