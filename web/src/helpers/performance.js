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

const DASH = '-';
const HOUR_SECONDS = 60 * 60;
export const MODEL_PERFORMANCE_HOURS = 24;

function isPositiveFinite(value) {
  return Number.isFinite(value) && value > 0;
}

function formatNumber(value, options) {
  return new Intl.NumberFormat(undefined, options).format(value);
}

export function formatRequestCount(value) {
  if (!Number.isFinite(value) || value < 0) return DASH;
  return formatNumber(value, { maximumFractionDigits: 0 });
}

export function formatTps(value) {
  if (!isPositiveFinite(value)) return DASH;
  return `${formatNumber(value, { maximumFractionDigits: 2 })} TPS`;
}

export function formatLatency(value) {
  if (!isPositiveFinite(value)) return DASH;
  if (value < 1000) {
    return `${formatNumber(value, { maximumFractionDigits: 0 })} ms`;
  }
  return `${formatNumber(value / 1000, { maximumFractionDigits: 2 })} s`;
}

export function formatPercentage(value) {
  if (!Number.isFinite(value)) return DASH;
  const clamped = Math.min(100, Math.max(0, value));
  return `${formatNumber(clamped, { maximumFractionDigits: 2 })}%`;
}

function formatCompactNumber(value) {
  if (!isPositiveFinite(value)) return DASH;
  return value > 1 ? String(Math.round(value)) : value.toFixed(1);
}

export function formatCompactLatency(ms) {
  if (!isPositiveFinite(ms)) return DASH;
  if (ms >= 1000) {
    return `${formatCompactNumber(ms / 1000)}s`;
  }
  return `${formatCompactNumber(ms)}ms`;
}

export function formatCompactThroughput(tps) {
  if (!isPositiveFinite(tps)) return DASH;
  if (tps >= 1000) {
    return `${formatCompactNumber(tps / 1000)}Kt`;
  }
  return `${formatCompactNumber(tps)}t`;
}

export function getSuccessRateLevel(rate) {
  if (!Number.isFinite(rate)) return 'unknown';
  if (rate >= 100) return 'excellent';
  if (rate >= 90) return 'good';
  if (rate >= 70) return 'warning';
  return 'critical';
}

const SUCCESS_RATE_COLORS = {
  excellent: '#10b981',
  good: '#34d399',
  warning: '#f59e0b',
  critical: '#ef4444',
  unknown: 'var(--semi-color-fill-2)',
};

export function getSuccessRateColor(rate) {
  return SUCCESS_RATE_COLORS[getSuccessRateLevel(rate)];
}

export function getUptimeBarHeight(rate, hasData = true) {
  if (!hasData || !Number.isFinite(rate)) return '20%';
  if (rate >= 99.9) return '100%';
  if (rate >= 99) return '88%';
  if (rate >= 95) return '72%';
  if (rate >= 90) return '55%';
  return '40%';
}

export function normalizeRecentSuccessRates(rates = []) {
  const recent = rates.map(Number).filter(Number.isFinite).slice(-3);
  return [...Array(Math.max(0, 3 - recent.length)).fill(null), ...recent];
}

export function formatChartTime(timestamp, hours) {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return DASH;
  const date = new Date(timestamp * 1000);
  const options =
    hours === 1 || hours === MODEL_PERFORMANCE_HOURS
      ? { hour: '2-digit', minute: '2-digit', hour12: false }
      : {
          month: '2-digit',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        };
  return new Intl.DateTimeFormat(undefined, options).format(date);
}

export function normalizeHourlyPerformanceSeries(
  series = [],
  nowMs = Date.now(),
) {
  const endTs =
    Math.floor(Math.floor(nowMs / 1000) / HOUR_SECONDS) * HOUR_SECONDS;
  const pointsByHour = new Map();

  for (const point of series) {
    const timestamp = Number(point?.ts);
    if (!Number.isFinite(timestamp) || timestamp <= 0) continue;
    const hourTs = Math.floor(timestamp / HOUR_SECONDS) * HOUR_SECONDS;
    pointsByHour.set(hourTs, point);
  }

  return Array.from({ length: MODEL_PERFORMANCE_HOURS }, (_, index) => {
    const ts = endTs - (MODEL_PERFORMANCE_HOURS - 1 - index) * HOUR_SECONDS;
    const point = pointsByHour.get(ts);
    const requestCount = Number(point?.request_count);
    const hasData = Number.isFinite(requestCount) && requestCount > 0;

    return {
      ...(point || {}),
      ts,
      request_count: hasData ? requestCount : 0,
      avg_ttft_ms:
        hasData && Number(point.avg_ttft_ms) > 0
          ? Number(point.avg_ttft_ms)
          : null,
      avg_latency_ms: hasData ? Number(point.avg_latency_ms) : null,
      success_rate:
        hasData && Number.isFinite(Number(point.success_rate))
          ? Number(point.success_rate)
          : 0,
      avg_tps: hasData ? Number(point.avg_tps) : null,
      has_data: hasData,
    };
  });
}

export function countPerformanceIncidents(series = []) {
  return series.filter((point) => {
    const requestCount = Number(point?.request_count);
    const successRate = Number(point?.success_rate);
    return (
      Number.isFinite(requestCount) &&
      requestCount > 0 &&
      Number.isFinite(successRate) &&
      successRate < 100
    );
  }).length;
}

export function buildPerformanceChartSeries(groups = []) {
  const latencyByTs = new Map();
  const availabilityByTs = new Map();

  for (const group of groups) {
    for (const point of group?.series || []) {
      const ts = Number(point?.ts);
      if (!Number.isFinite(ts) || ts <= 0) continue;

      const ttft = Number(point?.avg_ttft_ms);
      if (Number.isFinite(ttft) && ttft >= 0) {
        const values = latencyByTs.get(ts) || [];
        values.push(ttft);
        latencyByTs.set(ts, values);
      }

      const successRate = Number(point?.success_rate);
      if (Number.isFinite(successRate)) {
        const values = availabilityByTs.get(ts) || [];
        values.push(Math.min(100, Math.max(0, successRate)));
        availabilityByTs.set(ts, values);
      }
    }
  }

  const averageEntries = (entries, valueKey) =>
    Array.from(entries.entries())
      .sort(([left], [right]) => left - right)
      .map(([ts, values]) => ({
        ts,
        [valueKey]:
          values.reduce((sum, value) => sum + value, 0) / values.length,
      }));

  return {
    latency: averageEntries(latencyByTs, 'avg_ttft_ms'),
    availability: averageEntries(availabilityByTs, 'success_rate'),
  };
}
