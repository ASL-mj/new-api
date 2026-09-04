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

import { describe, expect, test, mock } from 'bun:test';

// api.js 间接引入 Semi UI 与 utils.jsx，lottie-web 和 window.matchMedia
// 都在模块加载期访问浏览器 API；bun test 没有 DOM，先打桩避免加载报错。
mock.module('lottie-web', () => ({ default: {} }));
const storageStub = () => {
  const store = new Map();
  return {
    getItem: (key) => (store.has(key) ? store.get(key) : null),
    setItem: (key, value) => store.set(key, String(value)),
    removeItem: (key) => store.delete(key),
    clear: () => store.clear(),
  };
};
globalThis.window ??= {
  matchMedia: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }),
  location: { href: 'http://localhost/' },
  navigator: { userAgent: 'bun-test' },
  btoa: (input) => globalThis.btoa(input),
  atob: (input) => globalThis.atob(input),
};
globalThis.localStorage ??= storageStub();

const { buildApiPayload, processModelsData } = await import('../api');

describe('playground model and reasoning payloads', () => {
  test('sends the selected model as-is without mapping', () => {
    const payload = buildApiPayload(
      [],
      '',
      {
        model: 'gpt-4o',
        group: 'default',
        stream: true,
      },
      {},
    );

    expect(payload.model).toBe('gpt-4o');
  });

  test('selects the first model when the current model is unavailable', () => {
    const { selectedModel } = processModelsData(
      ['first-model', 'second-model'],
      'model-from-another-group',
    );

    expect(selectedModel).toBe('first-model');
  });

  test('includes an explicitly selected reasoning effort', () => {
    const payload = buildApiPayload(
      [],
      '',
      {
        model: 'origin-id',
        group: 'default',
        stream: true,
        reasoning_effort: 'xhigh',
      },
      {},
    );

    expect(payload).toMatchObject({
      model: 'origin-id',
      reasoning_effort: 'xhigh',
    });
  });

  test('omits the system-default reasoning effort', () => {
    const payload = buildApiPayload(
      [],
      '',
      {
        model: 'origin-id',
        group: 'default',
        stream: true,
        reasoning_effort: '',
      },
      {},
    );

    expect(payload).not.toHaveProperty('reasoning_effort');
  });
});
