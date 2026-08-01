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
  getCommonChannelModels,
  normalizeMonitorGroupPayload,
  parseChannelModels,
} from './monitorGroupUtils.js';

describe('monitor group model helpers', () => {
  test('parses channel models in source order and removes blanks and duplicates', () => {
    expect(parseChannelModels(' gpt-5.4, gpt-5.4-mini,,gpt-5.4 ')).toEqual([
      'gpt-5.4',
      'gpt-5.4-mini',
    ]);
  });

  test('returns only models supported by every selected channel', () => {
    expect(
      getCommonChannelModels([
        { models: 'gpt-5.4,gpt-5.4-mini,gpt-5.5' },
        { models: 'gpt-5.5,gpt-5.4' },
      ]),
    ).toEqual(['gpt-5.4', 'gpt-5.5']);
    expect(
      getCommonChannelModels([{ models: 'gpt-5.4' }, { models: '' }]),
    ).toEqual([]);
  });

  test('normalizes ids and excludes the primary model from extra models', () => {
    expect(
      normalizeMonitorGroupPayload(
        {
          name: ' Core ',
          key: 'changed',
          primary_model: 'gpt-5.4',
          extra_models: ['gpt-5.4-mini', 'gpt-5.4', 'gpt-5.4-mini'],
          channel_ids: ['2', 1, '2'],
        },
        { id: 9, key: 'stable-key' },
      ),
    ).toMatchObject({
      id: 9,
      name: 'Core',
      key: 'stable-key',
      primary_model: 'gpt-5.4',
      extra_models: ['gpt-5.4-mini'],
      channel_ids: [2, 1],
    });
  });
});
