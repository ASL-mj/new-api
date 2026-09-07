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

export const H3_RATIOS = [
  'adaptive',
  '21:9',
  '16:9',
  '4:3',
  '1:1',
  '3:4',
  '9:16',
];
export const H3_MEDIA = {
  first_frame: 'image_url',
  last_frame: 'image_url',
  reference_image: 'image_url',
  reference_video: 'video_url',
  reference_audio: 'audio_url',
};

export const safeTaskUrl = (value) => {
  if (typeof value !== 'string') return '';
  try {
    const url = new URL(value);
    return ['https:', 'http:'].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? url.href
      : '';
  } catch {
    return '';
  }
};

export const taskDetailHref = (taskId) => {
  const normalizedTaskId =
    typeof taskId === 'string' ? taskId.trim() : String(taskId || '').trim();
  return normalizedTaskId
    ? `/console/task?task_id=${encodeURIComponent(normalizedTaskId)}`
    : '/console/task';
};

export const buildH3Submission = (values) => {
  if (!Number.isSafeInteger(values.token_id) || values.token_id <= 0)
    throw new Error('请选择有效令牌');
  const prompt = String(values.prompt || '').trim();
  if (!prompt) throw new Error('请输入提示词');
  if (typeof values.model !== 'string' || !values.model.trim())
    throw new Error('请选择有效视频模型');
  if (
    !Number.isInteger(values.duration) ||
    values.duration < 4 ||
    values.duration > 15
  ) {
    throw new Error('视频时长须为 4 至 15 秒的整数');
  }
  if (!['768P', '2K'].includes(values.size))
    throw new Error('请选择有效分辨率');
  const media = values.media || [];
  if (!Array.isArray(media)) throw new Error('请选择有效素材类型');
  const counts = {};
  for (const item of media) {
    if (!item || !Object.prototype.hasOwnProperty.call(H3_MEDIA, item.role))
      throw new Error('请选择有效素材类型');
    if (!safeTaskUrl(item.url))
      throw new Error('素材须为不含凭据的 HTTP(S) URL');
    counts[item.role] = (counts[item.role] || 0) + 1;
  }
  const frames = (counts.first_frame || 0) + (counts.last_frame || 0);
  const references =
    (counts.reference_image || 0) +
    (counts.reference_video || 0) +
    (counts.reference_audio || 0);
  if (frames && references) throw new Error('首尾帧与参考素材须分开提交');
  if ((counts.first_frame || 0) > 1 || (counts.last_frame || 0) > 1)
    throw new Error('首帧和尾帧各限一张');
  if (
    frames + (counts.reference_image || 0) > 9 ||
    (counts.reference_video || 0) > 3 ||
    (counts.reference_audio || 0) > 3
  ) {
    throw new Error('图片最多 9 张，视频和音频各最多 3 个');
  }
  const hasVisual = media.some((item) => H3_MEDIA[item.role] !== 'audio_url');
  const ratio =
    !values.ratio || values.ratio === 'auto'
      ? hasVisual
        ? 'adaptive'
        : '16:9'
      : values.ratio;
  if (!H3_RATIOS.includes(ratio) || (ratio === 'adaptive' && !hasVisual)) {
    throw new Error('自适应比例需要图片或视频素材');
  }
  return {
    token_id: values.token_id,
    model: values.model,
    prompt,
    duration: values.duration,
    size: values.size,
    metadata: {
      ratio,
      content: [
        { type: 'text', text: prompt },
        ...media.map((item) => {
          const type = H3_MEDIA[item.role];
          return { type, role: item.role, [type]: { url: item.url.trim() } };
        }),
      ],
    },
  };
};

export const h3TokenOptions = (items, now = Date.now() / 1000) =>
  items.map((item) => ({
    value: item.id,
    label: item.name || String(item.id),
    disabled:
      item.status !== 1 ||
      (item.expired_time > 0 && item.expired_time <= now) ||
      (!item.unlimited_quota && item.remain_quota <= 0),
  }));

export const H3_PRICE_KEY = 'minimax_h3_pricing.rules';
export const validateH3Pricing = (rule) => {
  const positive = (value) =>
    typeof value === 'number' && Number.isFinite(value) && value > 0;
  const nonnegative = (value) =>
    typeof value === 'number' && Number.isFinite(value) && value >= 0;
  if (
    !rule ||
    typeof rule.version !== 'string' ||
    !rule.version.trim() ||
    new TextEncoder().encode(rule.version).length > 128
  )
    throw new Error('请填写价格版本');
  if (!Number.isSafeInteger(rule.base_quota) || rule.base_quota <= 0)
    throw new Error('基础额度须为正整数');
  if (
    !positive(rule.duration_base_seconds) ||
    !positive(rule.resolution_ratios?.['768P']) ||
    !positive(rule.resolution_ratios?.['2K']) ||
    Object.keys(rule.resolution_ratios).length !== 2
  )
    throw new Error('请填写有效时长基数和分辨率倍率');
  if (
    ![
      'image_reference_ratio_per_image',
      'video_reference_ratio_per_second',
      'audio_reference_ratio_per_audio',
    ].every((key) => nonnegative(rule[key]))
  )
    throw new Error('素材倍率须为非负数');
  return { ...rule, version: rule.version.trim() };
};

// Do not infer settled amounts from the mutable legacy task quota.
export const taskBillingSummary = (task) => {
  const validQuota = (value) =>
    typeof value === 'number' &&
    Number.isFinite(value) &&
    Number.isInteger(value);
  const applied = task.settlement_status === 'applied';
  const delta =
    applied && validQuota(task.quota_delta) ? task.quota_delta : null;
  return {
    precharge: validQuota(task.pre_consumed_quota)
      ? task.pre_consumed_quota
      : null,
    final:
      applied && task.settlement_kind === 'refund'
        ? 0
        : applied && validQuota(task.actual_quota)
          ? task.actual_quota
          : null,
    refund: delta !== null && delta < 0 ? -delta : null,
    additional: delta !== null && delta > 0 ? delta : null,
  };
};

export const taskDiagnosticFields = (
  task,
  { admin = false, root = false } = {},
) => {
  const keys =
    admin || root
      ? [
          'poll_failures',
          'last_poll_error_class',
          'last_poll_error_message',
          'last_poll_at',
          'next_poll_at',
        ]
      : [];
  if (root) keys.push('settlement_id', 'pricing_version');
  return Object.fromEntries(
    keys.filter((key) => task[key] != null).map((key) => [key, task[key]]),
  );
};

export const readTaskBillingLog = (other) => {
  try {
    const data = typeof other === 'string' ? JSON.parse(other) : other;
    const billing = data?.task_billing;
    return billing && typeof billing === 'object' && !Array.isArray(billing)
      ? billing
      : null;
  } catch {
    return null;
  }
};
