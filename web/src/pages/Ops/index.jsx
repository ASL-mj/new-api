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

import React, { useEffect } from 'react';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';

import { useOpsData } from '../../hooks/ops/useOpsData';
import OpsAlerts from './components/OpsAlerts';
import OpsDetailModal from './components/OpsDetailModal';
import OpsHeader from './components/OpsHeader';
import OpsOverviewPanel from './components/OpsOverviewPanel';
import OpsRankingsPanel from './components/OpsRankingsPanel';
import OpsSystemStatus from './components/OpsSystemStatus';
import OpsTrendPanel from './components/OpsTrendPanel';
import SystemEventLogPanel from './components/SystemEventLogPanel';

const Ops = () => {
  const data = useOpsData();

  useEffect(() => {
    initVChartSemiTheme({ isWatchingThemeSwitch: true });
  }, []);

  return (
    <div className='mt-[60px] px-2'>
      <OpsHeader {...data} />
      <OpsOverviewPanel {...data} />
      <OpsSystemStatus {...data} />
      <div className='grid grid-cols-1 gap-3 xl:grid-cols-12'>
        <div className='xl:col-span-5'>
          <OpsTrendPanel overview={data.overview} t={data.t} />
        </div>
        <div className='xl:col-span-4'>
          <OpsRankingsPanel {...data} />
        </div>
        <div className='xl:col-span-3'>
          <OpsAlerts
            alerts={data.overview?.recent_alerts || []}
            openEventDetail={data.openEventDetail}
            t={data.t}
          />
        </div>
      </div>
      <SystemEventLogPanel {...data} />
      <OpsDetailModal {...data} />
    </div>
  );
};

export default Ops;
