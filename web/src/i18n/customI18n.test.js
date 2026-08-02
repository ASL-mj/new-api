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

import en from './locales/en.json';
import fr from './locales/fr.json';
import ja from './locales/ja.json';
import ru from './locales/ru.json';
import vi from './locales/vi.json';
import zhCN from './locales/zh-CN.json';
import zhTW from './locales/zh-TW.json';
import { SKYE_CUSTOM_I18N_KEYS } from './customKeys';

const locales = { en, fr, ja, ru, vi, 'zh-CN': zhCN, 'zh-TW': zhTW };

const interpolationTokens = (value) =>
  [...String(value).matchAll(/{{\s*([^},\s]+)[^}]*}}/g)]
    .map((match) => match[1])
    .sort();

describe('Skye custom translations', () => {
  for (const [locale, resource] of Object.entries(locales)) {
    test(`${locale} contains every custom key`, () => {
      for (const key of SKYE_CUSTOM_I18N_KEYS) {
        const value = resource.translation[key];
        expect(value, `${locale}: ${key}`).toBeDefined();
        expect(String(value).trim(), `${locale}: ${key}`).not.toBe('');
        expect(interpolationTokens(value), `${locale}: ${key}`).toEqual(
          interpolationTokens(key),
        );
      }
    });
  }
});
