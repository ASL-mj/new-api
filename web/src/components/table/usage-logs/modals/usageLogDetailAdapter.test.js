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
import {
  buildUsageLogDetail,
  buildUsageLogBriefSummary,
} from './usageLogDetailAdapter';

const storage = new Map();

globalThis.localStorage = {
  clear: () => storage.clear(),
  getItem: (key) => (storage.has(key) ? storage.get(key) : null),
  removeItem: (key) => storage.delete(key),
  setItem: (key, value) => storage.set(key, String(value)),
};

const identityT = (key) => key;

const parseAmount = (text) => Number(String(text).replace(/[^0-9.]/g, ''));

const baseLog = {
  id: 101,
  type: 2,
  created_at: 1756400000,
  model_name: 'gpt-test',
  token_name: 'tok',
  group: 'default',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  other: '{}',
};

describe('usage log detail adapter', () => {
  beforeEach(() => {
    localStorage.clear();
    localStorage.setItem('quota_per_unit', '500000');
    localStorage.setItem('quota_display_type', 'USD');
  });

  test('builds standard ratio billing items consistent with the final quota', () => {
    // model_ratio 1 => $2/1M 输入；completion_ratio 2 => $4/1M 输出；cache_ratio 0.1 => $0.2/1M
    const record = {
      ...baseLog,
      prompt_tokens: 1000000,
      completion_tokens: 500000,
      quota: 0,
      other: JSON.stringify({
        model_ratio: 1,
        completion_ratio: 2,
        cache_ratio: 0.1,
        cache_tokens: 400000,
        group_ratio: 1,
      }),
    };
    const detail = buildUsageLogDetail({
      record,
      expandRows: [],
      t: identityT,
    });
    expect(detail.billing).not.toBeNull();
    expect(detail.billing.modeLabel).toBe('价格模式');

    const amounts = detail.billing.items.map((item) =>
      parseAmount(item.amount),
    );
    // 输入：600000 × $2/1M = $1.2；输出：500000 × $4/1M = $2；缓存读取：400000 × $0.2/1M = $0.08
    expect(Math.abs(amounts[0] - 1.2)).toBeLessThan(1e-6);
    // 单价为紧凑货币文本（每 1M Token），弹窗据此组装公式
    expect(detail.billing.items[0].tokens).toBe(600000);
    expect(detail.billing.items[0].unitPriceCompact).toBe('$2');
    expect(detail.billing.items[0].amount).toBe('$1.200000');
    expect(detail.billing.items[1].unitPriceCompact).toBe('$4');
    expect(detail.billing.items[2].unitPriceCompact).toBe('$0.2');
    expect(Math.abs(amounts[1] - 2)).toBeLessThan(1e-6);
    expect(Math.abs(amounts[2] - 0.08)).toBeLessThan(1e-6);

    // 分项合计 × 分组倍率 = 最终扣费
    const subtotal = detail.billing.items.reduce(
      (sum, item) => sum + parseAmount(item.amount),
      0,
    );
    expect(
      Math.abs(parseAmount(detail.billing.subtotalText) - subtotal),
    ).toBeLessThan(1e-6);
    expect(detail.billing.multiplierText).toBe('1x');
  });

  test('builds tiered billing items with auto-excluded prompt tokens', () => {
    // tier("short", p * 2 + c * 12 + cr * 0.2 + cc * 2.5)
    const expr = btoa('tier("short", p * 2 + c * 12 + cr * 0.2 + cc * 2.5)');
    const record = {
      ...baseLog,
      prompt_tokens: 169258,
      completion_tokens: 120,
      quota: 26439,
      other: JSON.stringify({
        billing_mode: 'tiered_expr',
        expr_b64: expr,
        matched_tier: 'short',
        cache_tokens: 159488,
        group_ratio: 1,
      }),
    };
    const detail = buildUsageLogDetail({
      record,
      expandRows: [],
      t: identityT,
    });
    expect(detail.billing.modeLabel).toBe('阶梯计费');
    expect(detail.billing.tierLabel).toBe('short');
    const labels = detail.billing.items.map((item) => item.label);
    expect(labels).toEqual(['输入', '输出', '缓存读取']);

    // p = 169258 - 159488 = 9770；cr = 159488；c = 120
    expect(detail.billing.items[0].tokens).toBe(9770);
    expect(detail.billing.items[1].tokens).toBe(120);
    expect(detail.billing.items[2].tokens).toBe(159488);

    // 分项合计换算回 quota 应接近实际扣费 26439
    const finalQuota = parseAmount(detail.billing.finalText) * 500000;
    expect(Math.abs(finalQuota - 26439)).toBeLessThan(2);
  });

  test('falls back to billing process text for unsupported billing types', () => {
    const record = {
      ...baseLog,
      prompt_tokens: 100,
      completion_tokens: 10,
      other: JSON.stringify({ claude: true, model_ratio: 1 }),
    };
    const expandRows = [{ key: '计费过程', value: 'claude-process' }];
    const detail = buildUsageLogDetail({ record, expandRows, t: identityT });
    expect(detail.billing).toBeNull();
    expect(detail.billingProcess).toBe('claude-process');
  });

  test('summarizes tokens, cache hit rate and stream speed', () => {
    const record = {
      ...baseLog,
      prompt_tokens: 1000,
      completion_tokens: 100,
      use_time: 10,
      is_stream: true,
      other: JSON.stringify({
        cache_tokens: 800,
        frt: 700,
        request_path: '/v1/chat/completions',
      }),
    };
    const detail = buildUsageLogDetail({
      record,
      expandRows: [],
      t: identityT,
    });
    expect(detail.tokens.input).toBe(1000);
    expect(detail.tokens.output).toBe(100);
    expect(detail.tokens.cacheRead).toBe(800);
    expect(detail.tokens.total).toBe(1100);
    expect(detail.tokens.hitRate).toBeCloseTo(80);
    expect(detail.speed).toBe(10);
    expect(detail.frtSeconds).toBe(0.7);
    expect(detail.requestPath).toBe('/v1/chat/completions');
  });

  test('handles empty and malformed log data without throwing', () => {
    const record = {
      ...baseLog,
      other: 'not-json',
      quota: undefined,
      request_id: '',
    };
    const detail = buildUsageLogDetail({
      record,
      expandRows: undefined,
      t: identityT,
    });
    expect(detail.requestId).toBe('');
    expect(detail.tokens.input).toBe(0);
    expect(detail.tokens.hitRate).toBe(0);
    expect(detail.billing).toBeNull();
  });

  test('returns null detail when record is missing', () => {
    expect(
      buildUsageLogDetail({ record: null, expandRows: [], t: identityT }),
    ).toBeNull();
  });

  test('builds one-line brief summary for the details column', () => {
    const standard = {
      ...baseLog,
      other: JSON.stringify({
        model_ratio: 1,
        completion_ratio: 2,
        cache_tokens: 400000,
      }),
    };
    expect(buildUsageLogBriefSummary(standard, identityT)).toBe(
      '价格 · $2 / $4/M +1',
    );

    const tiered = {
      ...baseLog,
      other: JSON.stringify({
        billing_mode: 'tiered_expr',
        expr_b64: btoa('tier("short", p * 2 + c * 12 + cr * 0.2)'),
        matched_tier: 'short',
      }),
    };
    expect(buildUsageLogBriefSummary(tiered, identityT)).toBe(
      '阶梯(short) · $2 / $12/M +1',
    );

    const errorLog = { ...baseLog, type: 5 };
    expect(buildUsageLogBriefSummary(errorLog, identityT)).toBe('错误详情');
  });
});
