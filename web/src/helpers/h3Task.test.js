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
  buildH3Submission,
  h3TokenOptions,
  readTaskBillingLog,
  safeTaskUrl,
  taskBillingSummary,
  taskDetailHref,
  taskDiagnosticFields,
  validateH3Pricing,
} from './h3Task';

const input = {
  token_id: 42,
  model: 'MiniMax-H3',
  prompt: ' city view ',
  duration: 5,
  size: '768P',
  ratio: 'auto',
};
const media = (role, count = 1) =>
  Array.from({ length: count }, (_, i) => ({
    role,
    url: 'https://media.example/' + i,
  }));
const rule = {
  version: 'h3-v1',
  base_quota: 1000,
  duration_base_seconds: 5,
  resolution_ratios: { '768P': 1, '2K': 2 },
  image_reference_ratio_per_image: 0,
  video_reference_ratio_per_second: 0.2,
  audio_reference_ratio_per_audio: 0,
};

describe('H3 dashboard submission', () => {
  test('builds a stable task-detail route without leaking extra query values', () => {
    expect(taskDetailHref(' task_1?x=2 ')).toBe(
      '/console/task?task_id=task_1%3Fx%3D2',
    );
    expect(taskDetailHref('')).toBe('/console/task');
  });
  test('text defaults and only token ID are sent', () => {
    const payload = buildH3Submission({
      ...input,
      key: 'SECRET',
      channel_api_key: 'SECRET',
    });
    expect(payload).toEqual({
      token_id: 42,
      model: 'MiniMax-H3',
      prompt: 'city view',
      duration: 5,
      size: '768P',
      metadata: {
        ratio: '16:9',
        content: [{ type: 'text', text: 'city view' }],
      },
    });
  });
  test('accepts a model returned for the selected token', () => {
    expect(
      buildH3Submission({ ...input, model: 'minimax_h3' }).model,
    ).toBe('minimax_h3');
  });
  test('frames and references map to explicit content roles', () => {
    const frames = buildH3Submission({
      ...input,
      media: [...media('first_frame'), ...media('last_frame')],
    });
    expect(frames.metadata.ratio).toBe('adaptive');
    expect(frames.metadata.content[1]).toEqual({
      type: 'image_url',
      role: 'first_frame',
      image_url: { url: 'https://media.example/0' },
    });
    const references = buildH3Submission({
      ...input,
      media: [
        ...media('reference_image', 9),
        ...media('reference_video', 3),
        ...media('reference_audio', 3),
      ],
    });
    expect(references.metadata.content).toHaveLength(16);
    expect(references.metadata.content[10].video_url.url).toBe(
      'https://media.example/0',
    );
  });
  test('audio alone uses 16:9 and rejects adaptive', () => {
    expect(
      buildH3Submission({ ...input, media: media('reference_audio') }).metadata
        .ratio,
    ).toBe('16:9');
    expect(() =>
      buildH3Submission({
        ...input,
        ratio: 'adaptive',
        media: media('reference_audio'),
      }),
    ).toThrow();
  });
  test.each([0, -1, null, '42', 1.5])('rejects token ID %p', (token_id) => {
    expect(() => buildH3Submission({ ...input, token_id })).toThrow();
  });
  test.each([3, 16, 4.5, '5', NaN])('rejects duration %p', (duration) => {
    expect(() => buildH3Submission({ ...input, duration })).toThrow();
  });
  test.each([
    { prompt: ' ' },
    { model: '' },
    { size: '1080P' },
    { ratio: 'bad' },
    { media: {} },
    { media: [{ role: 'constructor', url: 'https://media.example' }] },
  ])('rejects invalid input %p', (override) => {
    expect(() => buildH3Submission({ ...input, ...override })).toThrow();
  });
  test('rejects mixed/duplicate/oversize media', () => {
    for (const items of [
      [...media('first_frame'), ...media('reference_audio')],
      media('first_frame', 2),
      media('last_frame', 2),
      media('reference_image', 10),
      media('reference_video', 4),
      media('reference_audio', 4),
    ])
      expect(() => buildH3Submission({ ...input, media: items })).toThrow();
  });
  test.each([
    'javascript:alert(1)',
    'data:image/png;base64,abc',
    '/file.png',
    'https://user:password@media.example/asset',
  ])('rejects unsafe URL %s', (url) => {
    expect(safeTaskUrl(url)).toBe('');
    expect(() =>
      buildH3Submission({ ...input, media: [{ role: 'first_frame', url }] }),
    ).toThrow();
  });
  test('token options discard secrets and disable expired/unusable tokens', () => {
    const options = h3TokenOptions(
      [
        {
          id: 1,
          name: 'active',
          status: 1,
          unlimited_quota: true,
          expired_time: -1,
          key: 'SECRET',
        },
        {
          id: 2,
          name: 'expired',
          status: 1,
          remain_quota: 100,
          expired_time: 10,
        },
        { id: 3, status: 2, unlimited_quota: true },
        { id: 4, status: 1, remain_quota: 0 },
      ],
      20,
    );
    expect(options[0]).toEqual({ value: 1, label: 'active', disabled: false });
    expect(options.slice(1).every((option) => option.disabled)).toBe(true);
    expect(JSON.stringify(options)).not.toContain('SECRET');
  });
});

describe('task billing views', () => {
  test('matches dashboard null fields and applied zero refund projection', () => {
    const pending = {
      task_id: 'task_fixture',
      settlement_status: 'pending',
      pre_consumed_quota: null,
      actual_quota: null,
      quota_delta: null,
      settled_at: 0,
      data: null,
      properties: { origin_model_name: 'MiniMax-H3' },
    };
    expect(taskBillingSummary(pending)).toEqual({
      precharge: null,
      final: null,
      refund: null,
      additional: null,
    });
    const refund = {
      ...pending,
      settlement_status: 'applied',
      settlement_kind: 'refund',
      pre_consumed_quota: 500,
      actual_quota: 0,
      quota_delta: -500,
    };
    expect(taskBillingSummary(refund)).toEqual({
      precharge: 500,
      final: 0,
      refund: 500,
      additional: null,
    });
    expect(
      readTaskBillingLog(JSON.stringify({ task_billing: refund })),
    ).toEqual(refund);
  });
  test.each(['pending', 'applying', 'failed', 'legacy', undefined])(
    'does not call %p billing final',
    (settlement_status) => {
      expect(
        taskBillingSummary({
          settlement_status,
          quota: 1234,
          actual_quota: 200,
          quota_delta: -100,
          settlement_kind: 'refund',
        }),
      ).toEqual({
        precharge: null,
        final: null,
        refund: null,
        additional: null,
      });
    },
  );
  test('settled success and refunds preserve zero and negative delta semantics', () => {
    expect(
      taskBillingSummary({
        settlement_status: 'applied',
        settlement_kind: 'settle',
        pre_consumed_quota: 500,
        actual_quota: 400,
        quota_delta: -100,
      }),
    ).toEqual({ precharge: 500, final: 400, refund: 100, additional: null });
    expect(
      taskBillingSummary({
        settlement_status: 'applied',
        settlement_kind: 'refund',
        pre_consumed_quota: 500,
        actual_quota: null,
        quota_delta: -500,
      }).final,
    ).toBe(0);
    expect(
      taskBillingSummary({
        settlement_status: 'applied',
        settlement_kind: 'settle',
        actual_quota: 0,
        quota_delta: 100,
      }).additional,
    ).toBe(100);
  });
  test('role allowlists never include private data or root audit for admin', () => {
    const task = {
      poll_failures: 2,
      last_poll_error_message: 'redacted',
      settlement_id: 7,
      pricing_version: 'v1',
      private_data: { key: 'SECRET' },
      channel_api_key: 'SECRET',
    };
    expect(taskDiagnosticFields(task)).toEqual({});
    expect(taskDiagnosticFields(task, { admin: true })).toEqual({
      poll_failures: 2,
      last_poll_error_message: 'redacted',
    });
    expect(taskDiagnosticFields(task, { root: true })).toEqual({
      poll_failures: 2,
      last_poll_error_message: 'redacted',
      settlement_id: 7,
      pricing_version: 'v1',
    });
    expect(
      JSON.stringify(taskDiagnosticFields(task, { root: true })),
    ).not.toContain('SECRET');
  });
  test('only structured task billing is accepted from logs', () => {
    expect(readTaskBillingLog('{bad')).toBeNull();
    expect(readTaskBillingLog({ task_billing: [] })).toBeNull();
    expect(readTaskBillingLog({ task_billing: { task_id: 'task_1' } })).toEqual(
      { task_id: 'task_1' },
    );
  });
});

describe('H3 configured pricing', () => {
  test.each([undefined, null, {}, 0, ''])(
    'blank config %p is not a free price',
    (config) => {
      expect(() => validateH3Pricing(config)).toThrow();
    },
  );
  test('accepts explicitly zero surcharges', () =>
    expect(validateH3Pricing(rule)).toEqual(rule));
  test.each(Object.keys(rule))('requires field %s', (key) => {
    const partial = { ...rule };
    delete partial[key];
    expect(() => validateH3Pricing(partial)).toThrow();
  });
  test.each([
    { base_quota: 0 },
    { base_quota: 1.2 },
    { duration_base_seconds: Infinity },
    { resolution_ratios: { '768P': 1, '2K': 0 } },
    { image_reference_ratio_per_image: -1 },
    { video_reference_ratio_per_second: '0' },
    { version: ' ' },
  ])('rejects invalid pricing %p', (override) => {
    expect(() => validateH3Pricing({ ...rule, ...override })).toThrow();
  });
});
