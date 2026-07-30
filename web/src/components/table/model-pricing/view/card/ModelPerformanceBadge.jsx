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
  formatCompactLatency,
  formatCompactThroughput,
  getSuccessRateBarClass,
} from '../../../../../helpers/performance';

const ModelPerformanceBadge = ({ perf, t }) => {
  if (!perf) return null;

  const successRate = Number(perf.success_rate);
  const recentRates = (perf.recent_success_rates || []).filter((rate) =>
    Number.isFinite(rate),
  );
  const statusRates =
    recentRates.length > 0 ? recentRates.slice(-3) : [successRate];
  const statusBars = [
    ...Array(Math.max(0, 3 - statusRates.length)).fill(null),
    ...statusRates,
  ].slice(-3);
  const statusTitle = Number.isFinite(successRate)
    ? `${successRate.toFixed(1)}%`
    : '-';

  return (
    <div className='hidden w-[132px] shrink-0 grid-cols-[38px_48px_30px] gap-x-2 text-right tabular-nums min-[460px]:grid'>
      <div title={t('平均总延迟')}>
        <div className='text-[10px] leading-4 text-gray-400'>{t('延迟')}</div>
        <div className='whitespace-nowrap font-mono text-xs leading-4 text-gray-500'>
          {formatCompactLatency(perf.avg_latency_ms)}
        </div>
      </div>
      <div title={t('吞吐')}>
        <div className='truncate text-[10px] leading-4 text-gray-400'>
          {t('吞吐')}
        </div>
        <div className='whitespace-nowrap font-mono text-xs leading-4 text-gray-500'>
          {formatCompactThroughput(perf.avg_tps)}
        </div>
      </div>
      <div title={`${t('状态')}: ${statusTitle}`}>
        <div className='truncate text-[10px] leading-4 text-gray-400'>
          {t('状态')}
        </div>
        <div className='flex h-4 items-center justify-end gap-0.5'>
          {statusBars.map((rate, index) => (
            <span
              key={`${index}-${rate ?? 'empty'}`}
              className={`w-1 rounded-full ${index === 0 ? 'h-2' : index === 1 ? 'h-2.5' : 'h-3'} ${rate == null ? 'bg-gray-200' : getSuccessRateBarClass(rate)}`}
            />
          ))}
        </div>
      </div>
    </div>
  );
};

export default ModelPerformanceBadge;
