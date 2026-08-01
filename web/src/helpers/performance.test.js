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

import { describe, expect, test } from 'bun:test';
import {
  buildPerformanceChartSeries,
  countPerformanceIncidents,
  formatCompactLatency,
  formatCompactThroughput,
  getSuccessRateColor,
  getSuccessRateLevel,
  getUptimeBarHeight,
  normalizeRecentSuccessRates,
  normalizeHourlyPerformanceSeries,
} from './performance';

describe('model card performance formatting', () => {
  test('formats compact latency and throughput', () => {
    expect(formatCompactLatency(0)).toBe('-');
    expect(formatCompactLatency(850)).toBe('850ms');
    expect(formatCompactLatency(1200)).toBe('1s');
    expect(formatCompactThroughput(9.6)).toBe('10t');
    expect(formatCompactThroughput(1000)).toBe('1.0Kt');
  });

  test('uses the exact success-rate thresholds', () => {
    expect(getSuccessRateLevel(100)).toBe('excellent');
    expect(getSuccessRateLevel(99.99)).toBe('good');
    expect(getSuccessRateLevel(90)).toBe('good');
    expect(getSuccessRateLevel(89.99)).toBe('warning');
    expect(getSuccessRateLevel(70)).toBe('warning');
    expect(getSuccessRateLevel(69.99)).toBe('critical');
    expect(getSuccessRateLevel(Number.NaN)).toBe('unknown');
    expect(getSuccessRateColor(100)).toBe('#10b981');
    expect(getSuccessRateColor(99)).toBe('#34d399');
    expect(getSuccessRateColor(80)).toBe('#f59e0b');
    expect(getSuccessRateColor(20)).toBe('#ef4444');
  });

  test('pads the latest three status buckets from the left', () => {
    expect(normalizeRecentSuccessRates([])).toEqual([null, null, null]);
    expect(normalizeRecentSuccessRates([100])).toEqual([null, null, 100]);
    expect(normalizeRecentSuccessRates([10, 20, 30, 40])).toEqual([
      20, 30, 40,
    ]);
  });

  test('uses the newer uptime severity heights', () => {
    expect(getUptimeBarHeight(100)).toBe('100%');
    expect(getUptimeBarHeight(99)).toBe('88%');
    expect(getUptimeBarHeight(95)).toBe('72%');
    expect(getUptimeBarHeight(90)).toBe('55%');
    expect(getUptimeBarHeight(50)).toBe('40%');
    expect(getUptimeBarHeight(null, false)).toBe('20%');
  });

  test('normalizes a sparse series into 24 hourly positions', () => {
    const now = Date.UTC(2026, 7, 1, 12, 34) - 17 * 60 * 1000;
    const endTs = Math.floor(now / 1000 / 3600) * 3600;
    const series = normalizeHourlyPerformanceSeries(
      [
        {
          ts: endTs - 2 * 3600,
          request_count: 2,
          avg_ttft_ms: 120,
          avg_latency_ms: 800,
          success_rate: 50,
          avg_tps: 12,
        },
      ],
      now,
    );

    expect(series).toHaveLength(24);
    expect(series[21].request_count).toBe(2);
    expect(series[21].avg_latency_ms).toBe(800);
    expect(series[20].request_count).toBe(0);
    expect(series[20].avg_latency_ms).toBeNull();
    expect(series[20].success_rate).toBe(0);
    expect(series[20].avg_ttft_ms).toBeNull();
    expect(series[23].ts).toBe(endTs);
  });

  test('does not count empty hours as incidents', () => {
    expect(
      countPerformanceIncidents([
        { request_count: 0, success_rate: 0 },
        { request_count: 4, success_rate: 100 },
        { request_count: 2, success_rate: 75 },
      ]),
    ).toBe(1);
  });

  test('keeps all hourly buckets when building chart points like newer NewAPI', () => {
    const charts = buildPerformanceChartSeries([
      {
        series: [
          { ts: 100, request_count: 0, avg_ttft_ms: 0, success_rate: 0 },
          { ts: 200, request_count: 2, avg_ttft_ms: 1000, success_rate: 80 },
        ],
      },
      {
        series: [
          { ts: 200, request_count: 3, avg_ttft_ms: 3000, success_rate: 100 },
          { ts: 300, request_count: 1, avg_ttft_ms: 500, success_rate: 50 },
        ],
      },
    ]);

    expect(charts.latency).toEqual([
      { ts: 100, avg_ttft_ms: 0 },
      { ts: 200, avg_ttft_ms: 2000 },
      { ts: 300, avg_ttft_ms: 500 },
    ]);
    expect(charts.availability).toEqual([
      { ts: 100, success_rate: 0 },
      { ts: 200, success_rate: 90 },
      { ts: 300, success_rate: 50 },
    ]);
  });
});
