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

import { beforeEach, describe, expect, test } from 'bun:test';
import { getCurrencyConfig } from './currency';
import { displayAmountToQuota, quotaToDisplayAmount } from './channelQuota';

const storage = new Map();

globalThis.localStorage = {
  clear: () => storage.clear(),
  getItem: (key) => (storage.has(key) ? storage.get(key) : null),
  removeItem: (key) => storage.delete(key),
  setItem: (key, value) => storage.set(key, String(value)),
};

const setQuotaDisplay = ({ type, quotaPerUnit = 500000, status = {} }) => {
  localStorage.setItem('quota_display_type', type);
  localStorage.setItem('quota_per_unit', quotaPerUnit);
  localStorage.setItem('status', JSON.stringify(status));
};

describe('channel quota amount conversion', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test('converts USD amounts with QuotaPerUnit 500000', () => {
    setQuotaDisplay({ type: 'USD' });

    expect(quotaToDisplayAmount(1250000)).toBe(2.5);
    expect(displayAmountToQuota(2.5)).toBe(1250000);
  });

  test('converts CNY amounts using the configured USD exchange rate', () => {
    setQuotaDisplay({
      type: 'CNY',
      status: { usd_exchange_rate: 7.2 },
    });

    expect(quotaToDisplayAmount(500000)).toBe(7.2);
    expect(displayAmountToQuota(14.4)).toBe(1000000);
  });

  test('converts custom currency amounts using the custom exchange rate', () => {
    setQuotaDisplay({
      type: 'CUSTOM',
      status: {
        custom_currency_exchange_rate: 8,
        custom_currency_symbol: 'S',
      },
    });

    expect(quotaToDisplayAmount(500000)).toBe(8);
    expect(displayAmountToQuota(4)).toBe(250000);
  });

  test('keeps TOKENS display in the native integer quota unit', () => {
    setQuotaDisplay({ type: 'TOKENS' });

    expect(getCurrencyConfig().symbol).toBe('');
    expect(quotaToDisplayAmount(123456)).toBe(123456);
    expect(displayAmountToQuota(123456.4)).toBe(123456);
  });

  test('maps zero and invalid values to zero', () => {
    setQuotaDisplay({ type: 'USD' });

    expect(quotaToDisplayAmount(0)).toBe(0);
    expect(quotaToDisplayAmount('invalid')).toBe(0);
    expect(quotaToDisplayAmount(Number.POSITIVE_INFINITY)).toBe(0);
    expect(displayAmountToQuota(0)).toBe(0);
    expect(displayAmountToQuota('invalid')).toBe(0);
    expect(displayAmountToQuota(-1)).toBe(0);
  });

  test('falls back to the stable CNY exchange rate for invalid values', () => {
    const invalidCases = [
      { usd_exchange_rate: 0 },
      { usd_exchange_rate: -2 },
      { usd_exchange_rate: 'NaN' },
      { usd_exchange_rate: 'Infinity' },
    ];

    for (const status of invalidCases) {
      setQuotaDisplay({
        type: 'CNY',
        status,
      });

      expect(getCurrencyConfig().rate).toBe(7);
      expect(quotaToDisplayAmount(500000)).toBe(7);
      expect(displayAmountToQuota(14)).toBe(1000000);
    }
  });

  test('falls back to the stable custom exchange rate for invalid values', () => {
    const invalidCases = [
      { custom_currency_exchange_rate: 0, custom_currency_symbol: 'S' },
      { custom_currency_exchange_rate: -2, custom_currency_symbol: 'S' },
      { custom_currency_exchange_rate: 'NaN', custom_currency_symbol: 'S' },
      {
        custom_currency_exchange_rate: 'Infinity',
        custom_currency_symbol: 'S',
      },
    ];

    for (const status of invalidCases) {
      setQuotaDisplay({
        type: 'CUSTOM',
        status,
      });

      expect(getCurrencyConfig().rate).toBe(1);
      expect(quotaToDisplayAmount(500000)).toBe(1);
      expect(displayAmountToQuota(2)).toBe(1000000);
    }
  });
});
