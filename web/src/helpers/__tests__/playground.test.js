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
  buildApiPayload,
  normalizeModelValue,
  processModelsData,
} from '../api';

describe('playground model and reasoning payloads', () => {
  test('keeps model value separate from its display label', () => {
    const { modelOptions, selectedModel } = processModelsData(
      [{ label: 'Mapped name', value: 'origin-id' }],
      'origin-id',
    );

    expect(modelOptions).toEqual([
      { label: 'Mapped name', value: 'origin-id' },
    ]);
    expect(selectedModel).toBe('origin-id');
  });

  test('converts a legacy display label back to the public model value', () => {
    const options = [{ label: 'hy3', value: 'b/hy3' }];

    expect(normalizeModelValue('hy3', options)).toBe('b/hy3');
    expect(processModelsData(options, 'hy3').selectedModel).toBe('b/hy3');
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

  test('normalizes the model in the final request payload', () => {
    const payload = buildApiPayload(
      [],
      '',
      {
        model: 'hy3',
        group: 'auto',
        stream: true,
      },
      {},
      [{ label: 'hy3', value: 'b/hy3' }],
    );

    expect(payload.model).toBe('b/hy3');
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
