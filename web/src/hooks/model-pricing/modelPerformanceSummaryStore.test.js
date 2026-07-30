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
import { createModelPerformanceSummaryStore } from './modelPerformanceSummaryStore';

describe('model performance summary store', () => {
  test('deduplicates in-flight requests and caches by hours for 60 seconds', async () => {
    let now = 1000;
    let calls = 0;
    const store = createModelPerformanceSummaryStore({
      now: () => now,
      fetchSummary: async () => {
        calls += 1;
        return { models: [{ model_name: 'gpt-5.4', avg_latency_ms: 12 }] };
      },
    });

    const first = store.load(24);
    const second = store.load(24);
    expect(first).toBe(second);

    const models = await first;
    expect(calls).toBe(1);
    expect(models.get('gpt-5.4').avg_latency_ms).toBe(12);

    now += 59999;
    await store.load(24);
    expect(calls).toBe(1);

    now += 2;
    await store.load(24);
    expect(calls).toBe(2);
  });

  test('does not cache rejected requests', async () => {
    let calls = 0;
    const store = createModelPerformanceSummaryStore({
      now: () => 1000,
      fetchSummary: async () => {
        calls += 1;
        if (calls === 1) {
          throw new Error('network');
        }
        return { models: [] };
      },
    });

    await expect(store.load(24)).rejects.toThrow('network');
    expect(await store.load(24)).toEqual(new Map());
    expect(calls).toBe(2);
  });
});
