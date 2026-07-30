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
  formatCompactLatency,
  formatCompactThroughput,
  getSuccessRateLevel,
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
  });
});
