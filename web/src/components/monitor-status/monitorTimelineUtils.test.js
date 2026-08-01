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
  MONITOR_TIMELINE_SIZE,
  getMonitorProviderIconName,
  monitorTimelineHeight,
  padMonitorTimeline,
  readMonitorStatusView,
  writeMonitorStatusView,
} from './monitorTimelineUtils.js';

const createStorage = (initial = {}) => {
  const values = new Map(Object.entries(initial));
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
};

describe('monitor status timeline helpers', () => {
  test('pads an empty timeline with sixty empty points', () => {
    const result = padMonitorTimeline([]);
    expect(result).toHaveLength(MONITOR_TIMELINE_SIZE);
    expect(result.every((point) => point.status === 'empty')).toBe(true);
  });

  test('pads on the left and keeps only the newest sixty points', () => {
    const thirty = Array.from({ length: 30 }, (_, index) => ({
      status: 'operational',
      checked_at: index,
    }));
    const padded = padMonitorTimeline(thirty);
    expect(padded.slice(0, 30).every((point) => point.status === 'empty')).toBe(
      true,
    );
    expect(padded.slice(30)).toEqual(thirty);

    const seventy = Array.from({ length: 70 }, (_, index) => ({
      status: 'degraded',
      checked_at: index,
    }));
    expect(padMonitorTimeline(seventy)[0].checked_at).toBe(10);
  });

  test('persists only supported card and list preferences', () => {
    const storage = createStorage();
    expect(readMonitorStatusView(storage)).toBe('card');
    writeMonitorStatusView('list', storage);
    expect(readMonitorStatusView(storage)).toBe('list');
    writeMonitorStatusView('invalid', storage);
    expect(readMonitorStatusView(storage)).toBe('list');
  });

  test('uses height as a second status signal', () => {
    expect(monitorTimelineHeight('operational')).toBe(100);
    expect(monitorTimelineHeight('degraded')).toBe(65);
    expect(monitorTimelineHeight('failed')).toBe(35);
    expect(monitorTimelineHeight('timeout')).toBe(35);
    expect(monitorTimelineHeight('empty')).toBe(15);
  });

  test('maps public channel labels to provider icons without channel ids', () => {
    expect(getMonitorProviderIconName(['OpenAI'])).toBe('OpenAI');
    expect(getMonitorProviderIconName(['Anthropic'])).toBe('Claude');
    expect(getMonitorProviderIconName(['Google Gemini'])).toBe('Gemini');
    expect(getMonitorProviderIconName(['Custom Gateway'])).toBe('Layers');
  });
});
