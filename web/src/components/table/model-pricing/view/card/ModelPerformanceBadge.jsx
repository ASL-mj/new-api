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
import { Activity, Gauge, Timer } from 'lucide-react';
import {
  formatCompactLatency,
  formatCompactThroughput,
  getSuccessRateColor,
  normalizeRecentSuccessRates,
} from '../../../../../helpers/performance';

const ModelPerformanceBadge = ({ perf, t }) => {
  const metrics = perf || {};
  const successRate = Number(metrics.success_rate);
  const statusBars = normalizeRecentSuccessRates(
    metrics.recent_success_rates || [],
  );
  const statusTitle = Number.isFinite(successRate)
    ? `${successRate.toFixed(1)}%`
    : '-';

  return (
    <div className='hidden w-[132px] shrink-0 grid-cols-[38px_48px_30px] gap-x-2 text-right tabular-nums min-[460px]:grid'>
      <div title={t('平均总延迟')}>
        <div className='flex items-center justify-end gap-0.5 text-[10px] leading-4 text-gray-400'>
          <Timer size={10} />
          {t('延迟')}
        </div>
        <div className='whitespace-nowrap font-mono text-xs leading-4 text-gray-500'>
          {formatCompactLatency(metrics.avg_latency_ms)}
        </div>
      </div>
      <div title={t('吞吐')}>
        <div className='flex items-center justify-end gap-0.5 truncate text-[10px] leading-4 text-gray-400'>
          <Gauge size={10} />
          {t('吞吐')}
        </div>
        <div className='whitespace-nowrap font-mono text-xs leading-4 text-gray-500'>
          {formatCompactThroughput(metrics.avg_tps)}
        </div>
      </div>
      <div
        title={
          perf ? `${t('状态')}: ${statusTitle}` : t('暂无性能数据')
        }
      >
        <div className='flex items-center justify-end gap-0.5 truncate text-[10px] leading-4 text-gray-400'>
          <Activity size={10} />
          {t('状态')}
        </div>
        <div className='flex h-4 items-center justify-end gap-0.5'>
          {statusBars.map((rate, index) => (
            <span
              key={`${index}-${rate ?? 'empty'}`}
              className='w-1 rounded-full'
              style={{
                height: `${8 + index * 2}px`,
                backgroundColor:
                  rate == null
                    ? 'var(--semi-color-fill-1)'
                    : getSuccessRateColor(rate),
              }}
            />
          ))}
        </div>
      </div>
    </div>
  );
};

export default ModelPerformanceBadge;
