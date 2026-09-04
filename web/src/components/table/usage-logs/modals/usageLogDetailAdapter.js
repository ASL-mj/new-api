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

// 详情适配层刻意只依赖纯函数模块（log/currency/billingExpr），
// 便于单元测试且不引入 UI 组件依赖；金额格式化与 renderQuota 口径一致。
import { getLogOther } from '../../../../helpers/log';
import { getCurrencyConfig } from '../../../../helpers/currency';
import { parseTiersFromExpr } from '../../../../helpers/billingExpr';
import { decodeFromBase64 } from '../../../../helpers/base64';

// 结构化计费行支持的费用变量（与后端阶梯计费变量语义一致，见 pkg/billingexpr/expr.md）
const TIERED_VAR_DEFS = [
  { key: 'p', field: 'inputPrice', labelKey: '输入' },
  { key: 'c', field: 'outputPrice', labelKey: '输出' },
  { key: 'cr', field: 'cacheReadPrice', labelKey: '缓存读取' },
  { key: 'cc', field: 'cacheCreatePrice', labelKey: '缓存创建' },
  { key: 'cc1h', field: 'cacheCreate1hPrice', labelKey: '1h缓存创建' },
];

const toNum = (value) => {
  const num = Number(value);
  return Number.isFinite(num) ? num : 0;
};

const toRatio = (value, fallback = 1) => {
  const num = Number(value);
  return Number.isFinite(num) ? num : fallback;
};

const getQuotaPerUnitSafe = () => {
  const raw = parseFloat(localStorage.getItem('quota_per_unit'));
  return Number.isFinite(raw) && raw > 0 ? raw : 500000;
};

// 与 helpers/render.jsx 的 renderQuota 同口径：quota → 当前展示货币文本
const renderQuota = (quota, digits = 2) => {
  const { symbol, rate, type } = getCurrencyConfig();
  if (type === 'TOKENS') {
    return Math.round(quota).toLocaleString();
  }
  return `${symbol}${((quota / getQuotaPerUnitSafe()) * rate).toFixed(digits)}`;
};

// 紧凑货币文本（去除末尾多余的 0），用于单价等简要展示：¥2、¥0.5、$3.75
const formatCompactPrice = (usdAmount) => {
  const { symbol, rate, type } = getCurrencyConfig();
  if (type === 'TOKENS') {
    return String(usdAmount);
  }
  return symbol + Number((usdAmount * rate).toFixed(6)).toString();
};

const formatTimestamp = (ts) => {
  if (!ts) {
    return '';
  }
  const date = new Date(ts * 1000);
  const pad = (n) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
};

const getEffectiveRatioInfo = (groupRatio, userGroupRatio, t) => {
  const parsed = Number(userGroupRatio);
  const useUserGroupRatio = Number.isFinite(parsed) && parsed !== -1;
  return {
    ratio: useUserGroupRatio ? parsed : toRatio(groupRatio, 1),
    label: useUserGroupRatio ? t('专属倍率') : t('分组倍率'),
    useUserGroupRatio,
  };
};

const formatFormulaAmount = (usdAmount) => {
  const quota = usdAmount * getQuotaPerUnitSafe();
  return renderQuota(quota, 6);
};

const formatCount = (value) => toNum(value).toLocaleString();

const isViolationFeeLog = (other) =>
  other?.violation_fee === true ||
  Boolean(other?.violation_fee_code) ||
  Boolean(other?.violation_fee_marker);

// System logs are only rendered as request-like rows when they were written by
// the channel probe recorder. Other type=4 records remain system records.
export const isMonitorProbeLog = (record) => {
  if (Number(record?.type) !== 4) {
    return false;
  }
  const other = getLogOther(record?.other);
  return other?.monitor_probe === true || other?.monitor_probe === 'true';
};

// 管理员调整用户额度的日志没有独立类型，后端以管理日志的 content 记录操作。
// 只识别额度调整语句，避免把其他管理操作误渲染成额度详情。
export const isAdminQuotaAdjustmentLog = (record) =>
  record?.type === 3 &&
  /管理员(?:增加|减少|覆盖)用户额度/.test(String(record.content || ''));

const getAdminQuotaAdjustmentContent = (content) =>
  String(content || '').replace(/^管理员(?=增加|减少|覆盖)/, '已');

const getLogType = (record) => Number(record?.type);

const hasRecordedValue = (value) =>
  value !== undefined && value !== null && value !== '' && value !== '-';

const overviewItem = (label, value, options = {}) =>
  hasRecordedValue(value) ? { label, value: String(value), ...options } : null;

const compactChannelName = (record) => {
  if (hasRecordedValue(record?.channel_name)) {
    return record.channel_name;
  }
  if (hasRecordedValue(record?.channel)) {
    return `#${record.channel}`;
  }
  return '';
};

const getDetailKind = (record, isQuotaAdjustment, isMonitorProbe) => {
  if (isQuotaAdjustment) {
    return 'quota-adjustment';
  }
  if (isMonitorProbe) {
    return 'probe';
  }
  switch (getLogType(record)) {
    case 1:
      return 'topup';
    case 2:
      return 'consume';
    case 3:
      return 'management';
    case 4:
      return 'system';
    case 5:
      return 'error';
    case 6:
      return 'refund';
    default:
      return 'legacy';
  }
};

const getDetailMeta = (kind, t) => {
  const metadata = {
    'quota-adjustment': {
      label: t('管理'),
      title: t('管理操作详情'),
      eventTitle: null,
    },
    consume: { label: t('消耗'), title: t('消耗详情'), eventTitle: null },
    error: {
      label: t('请求错误'),
      title: t('请求错误详情'),
      eventTitle: null,
    },
    topup: { label: t('充值'), title: t('充值详情'), eventTitle: t('充值信息') },
    management: {
      label: t('管理'),
      title: t('管理操作详情'),
      eventTitle: t('管理操作'),
    },
    system: {
      label: t('系统'),
      title: t('系统事件详情'),
      eventTitle: t('系统事件'),
    },
    probe: { label: t('探测'), title: t('探测详情'), eventTitle: null },
    refund: { label: t('退款'), title: t('退款详情'), eventTitle: t('退款信息') },
    legacy: {
      label: t('历史记录'),
      title: t('历史记录详情'),
      eventTitle: t('历史内容'),
    },
  };
  return metadata[kind] || metadata.legacy;
};

const buildOverviewItems = ({
  kind,
  record,
  createdAt,
  requestId,
  group,
  tokenName,
  modelName,
  upstreamModelName,
  reasoningEffort,
  useTime,
  requestPath,
  t,
}) => {
  const channelName = compactChannelName(record);
  const ip = record?.ip || '';
  const requestItem = overviewItem(t('请求 ID'), requestId, {
    copyable: true,
    mono: true,
    requestId: true,
  });
  const ipItem = overviewItem(t('IP'), ip, { copyable: true, mono: true });
  const timeItem = overviewItem(t('时间'), createdAt, { mono: true });
  const channelItem = overviewItem(t('渠道'), channelName, { mono: true });
  const pathItem = overviewItem(t('请求路径'), requestPath, {
    copyable: true,
    mono: true,
  });
  const responseTimeItem =
    useTime > 0
      ? overviewItem(t('响应时间'), `${useTime.toFixed(1)}s`, {
          mono: true,
          tone: 'success',
        })
      : null;

  if (kind === 'consume' || kind === 'probe') {
    return [
      requestItem,
      overviewItem(t('分组'), group, { mono: true }),
      overviewItem(t('令牌'), tokenName, { mono: true }),
      overviewItem(t('调用模型'), modelName, {
        mono: true,
        subValue: upstreamModelName ? `→ ${upstreamModelName}` : null,
      }),
      overviewItem(t('推理强度'), reasoningEffort),
      responseTimeItem,
      channelItem,
      ipItem,
      pathItem,
    ].filter(Boolean);
  }

  if (kind === 'error') {
    return [
      requestItem,
      overviewItem(t('分组'), group, { mono: true }),
      channelItem,
      overviewItem(t('调用模型'), modelName, {
        mono: true,
        subValue: upstreamModelName ? `→ ${upstreamModelName}` : null,
      }),
      overviewItem(t('令牌'), tokenName, { mono: true }),
      responseTimeItem,
      ipItem,
      pathItem,
    ].filter(Boolean);
  }

  return [
    timeItem,
    overviewItem(t('用户'), record?.username || '', { mono: true }),
    ipItem,
    overviewItem(t('日志 ID'), record?.id, { mono: true }),
  ].filter(Boolean);
};

const buildEventRows = (kind, record, other, t) => {
  const adminInfo =
    other?.admin_info && typeof other.admin_info === 'object'
      ? other.admin_info
      : {};
  const rows = [];
  const append = (label, value, options = {}) => {
    const item = overviewItem(label, value, options);
    if (item) {
      rows.push(item);
    }
  };

  if (kind === 'topup' || kind === 'refund') {
    if (Number(record?.quota) !== 0) {
      append(t('额度变化'), renderQuota(record.quota, 6), { mono: true });
    }
    append(t('任务 ID'), other?.task_id, { mono: true, copyable: true });
    append(t('订单支付方式'), adminInfo.payment_method);
    append(t('回调支付方式'), adminInfo.callback_payment_method);
    append(t('回调调用者IP'), adminInfo.caller_ip, {
      mono: true,
      copyable: true,
    });
    append(t('失败原因'), other?.reason);
  }

  if (kind === 'management') {
    const operator = adminInfo.admin_username
      ? adminInfo.admin_id
        ? `${adminInfo.admin_username} (ID: ${adminInfo.admin_id})`
        : adminInfo.admin_username
      : adminInfo.admin_id
        ? `ID: ${adminInfo.admin_id}`
        : '';
    append(t('操作管理员'), operator);
  }

  if (kind === 'system') {
    append(t('事件 ID'), other?.event_id, { mono: true, copyable: true });
  }

  if (kind === 'legacy') {
    append(t('日志类型'), record?.type, { mono: true });
  }

  return rows;
};

const buildErrorDetail = (record, other) => {
  const statusCode = other?.status_code ?? other?.upstream_status_code ?? null;
  const errorMessage =
    other?.error_message || other?.message || String(record?.content || '');

  return {
    statusCode,
    errorCode: other?.error_code || null,
    errorType: other?.error_type || null,
    stage: other?.error_stage || null,
    transportError: other?.transport_error || null,
    errorMessage: errorMessage || null,
  };
};

// 标准倍率计费：model_ratio * 2 = $X / 1M tokens（与 renderModelPrice 口径一致）
const buildStandardBillingItems = (record, other, t) => {
  const modelRatio = toNum(other.model_ratio);
  const completionRatio = toRatio(other.completion_ratio, 0);
  const cacheRatio = toRatio(other.cache_ratio, 1);
  const cacheReadTokens = toNum(other.cache_tokens);
  const inputTokens = toNum(record.prompt_tokens);
  const outputTokens = toNum(record.completion_tokens);
  const inputPrice = modelRatio * 2;
  const outputPrice = modelRatio * 2 * completionRatio;
  const cacheReadPrice = modelRatio * 2 * cacheRatio;
  const nonCacheInputTokens = Math.max(0, inputTokens - cacheReadTokens);

  const items = [];
  if (nonCacheInputTokens > 0) {
    items.push({
      label: t('输入'),
      unitPrice: inputPrice,
      unitPriceCompact: formatCompactPrice(inputPrice),
      tokens: nonCacheInputTokens,
      amount: formatFormulaAmount((nonCacheInputTokens / 1000000) * inputPrice),
      quota:
        (nonCacheInputTokens / 1000000) * inputPrice * getQuotaPerUnitSafe(),
    });
  }
  if (outputTokens > 0) {
    items.push({
      label: t('输出'),
      unitPrice: outputPrice,
      unitPriceCompact: formatCompactPrice(outputPrice),
      tokens: outputTokens,
      amount: formatFormulaAmount((outputTokens / 1000000) * outputPrice),
      quota: (outputTokens / 1000000) * outputPrice * getQuotaPerUnitSafe(),
    });
  }
  if (cacheReadTokens > 0) {
    items.push({
      label: t('缓存读取'),
      unitPrice: cacheReadPrice,
      unitPriceCompact: formatCompactPrice(cacheReadPrice),
      tokens: cacheReadTokens,
      amount: formatFormulaAmount((cacheReadTokens / 1000000) * cacheReadPrice),
      quota:
        (cacheReadTokens / 1000000) * cacheReadPrice * getQuotaPerUnitSafe(),
    });
  }
  return items;
};

// 阶梯表达式计费：表达式系数即 $/1M tokens 真实单价；
// p/c 自动排除单独计价的子类别（OpenAI 格式），Claude 格式不做减法。
const buildTieredBillingItems = (record, other, t) => {
  let exprStr = '';
  try {
    exprStr = decodeFromBase64(other.expr_b64);
  } catch (e) {
    return { items: null, tier: null };
  }
  const tiers = parseTiersFromExpr(exprStr);
  if (!tiers.length) {
    return { items: null, tier: null };
  }
  const tier =
    tiers.find((item) => item.label === other.matched_tier) || tiers[0];

  const pricedVars = TIERED_VAR_DEFS.filter(
    (def) => toNum(tier[def.field]) > 0,
  );
  if (
    pricedVars.some((def) => !['p', 'c', 'cr', 'cc', 'cc1h'].includes(def.key))
  ) {
    // 包含图片/音频等媒体变量时无法可靠还原分项 Token，退回现有计费过程展示
    return { items: null, tier };
  }
  const uses = (key) => pricedVars.some((def) => def.key === key);

  const isClaude = Boolean(other.claude);
  const inputTokens = toNum(record.prompt_tokens);
  const outputTokens = toNum(record.completion_tokens);
  const cacheReadTokens = toNum(other.cache_tokens);
  const cacheWrite5m = toNum(other.cache_creation_tokens_5m);
  const cacheWrite1h = toNum(other.cache_creation_tokens_1h);
  const cacheWriteTotal =
    cacheWrite5m + cacheWrite1h > 0
      ? cacheWrite5m + cacheWrite1h
      : toNum(other.cache_creation_tokens);

  const varTokens = {
    p: inputTokens,
    c: outputTokens,
    cr: cacheReadTokens,
    cc: cacheWrite5m > 0 ? cacheWrite5m : cacheWriteTotal,
    cc1h: cacheWrite1h,
  };
  if (!isClaude) {
    if (uses('cr')) varTokens.p -= cacheReadTokens;
    if (uses('cc')) varTokens.p -= varTokens.cc;
    if (uses('cc1h')) varTokens.p -= cacheWrite1h;
    varTokens.p = Math.max(0, varTokens.p);
  }

  const items = pricedVars
    .map((def) => {
      const price = toNum(tier[def.field]);
      const tokens = Math.max(0, varTokens[def.key]);
      // 非基础变量（缓存等）没有实际 Token 时不再输出 0 金额行，避免噪音
      if (tokens === 0 && !['p', 'c'].includes(def.key)) {
        return null;
      }
      return {
        label: t(def.labelKey),
        unitPrice: price,
        unitPriceCompact: formatCompactPrice(price),
        tokens,
        amount: formatFormulaAmount((tokens / 1000000) * price),
        quota: (tokens / 1000000) * price * getQuotaPerUnitSafe(),
      };
    })
    .filter(Boolean);
  return { items, tier };
};

// 结构化计费明细。无法可靠还原分项金额的计费类型返回 null，由弹窗退回现有计费过程展示。
const buildBilling = (record, other, t) => {
  // 违规扣费与订阅抵扣的结算语义不同，不套用 Token 计价公式
  if (isViolationFeeLog(other) || other?.billing_source === 'subscription') {
    return null;
  }
  const modelPrice = other?.model_price;
  let items = null;
  let modeLabel = null;
  let tierLabel = null;

  if (other?.billing_mode === 'tiered_expr' && other?.expr_b64) {
    modeLabel = t('阶梯计费');
    const tiered = buildTieredBillingItems(record, other, t);
    items = tiered.items;
    tierLabel = other.matched_tier || null;
  } else if (modelPrice != null && modelPrice !== -1) {
    modeLabel = t('按次计费');
    const priceUSD = toNum(modelPrice);
    items = [
      {
        label: t('按次'),
        unitPrice: priceUSD,
        unitPriceCompact: formatCompactPrice(priceUSD),
        tokens: 1,
        isPerRequest: true,
        amount: formatFormulaAmount(priceUSD),
        quota: priceUSD * getQuotaPerUnitSafe(),
      },
    ];
  } else if (
    other &&
    !other.is_task &&
    !other.ws &&
    !other.audio &&
    !other.image &&
    !other.claude
  ) {
    modeLabel = t('价格模式');
    items = buildStandardBillingItems(record, other, t);
  }

  // 没有实际 Token 消耗（如空数据或异常日志）时不输出结构化计费
  const hasConsumption =
    (Array.isArray(items) && items.some((item) => (item.quota || 0) > 0)) ||
    toNum(record.prompt_tokens) > 0 ||
    toNum(record.completion_tokens) > 0;
  if (!Array.isArray(items) || items.length === 0 || !hasConsumption) {
    return null;
  }

  const ratioInfo = getEffectiveRatioInfo(
    other?.group_ratio,
    other?.user_group_ratio,
    t,
  );
  const subtotalQuota = items.reduce((sum, item) => sum + (item.quota || 0), 0);
  return {
    modeLabel,
    tierLabel,
    items,
    subtotalText: renderQuota(subtotalQuota, 6),
    multiplierLabel: ratioInfo.label,
    multiplierText: `${ratioInfo.ratio}x`,
    finalText: renderQuota(record.quota || 0, 6),
  };
};

// 从现有展开数据中提取指定 key 的行，并移除已在结构化区域展示的行
const splitExpandRows = (expandRows, t) => {
  const rows = Array.isArray(expandRows) ? expandRows : [];
  const structuredKeys = new Set(
    [
      'Request ID',
      '请求路径',
      '缓存 Tokens',
      '缓存创建 Tokens',
      '计费过程',
      'Reasoning Effort',
      '请求并计费模型',
      '实际模型',
      '日志详情',
      '探测状态',
      'HTTP 状态码',
      '错误码',
      '错误类型',
      '错误信息',
      '标准口径',
    ].map((key) => t(key)),
  );
  const extraRows = rows.filter((row) => !structuredKeys.has(row.key));
  const findRow = (key) => rows.find((row) => row.key === t(key)) || null;
  return { extraRows, findRow };
};

export function buildUsageLogDetail({ record, expandRows, t }) {
  if (!record) {
    return null;
  }
  const other = getLogOther(record.other) || {};
  const { extraRows, findRow } = splitExpandRows(expandRows, t);
  const isQuotaAdjustment = isAdminQuotaAdjustmentLog(record);
  const isMonitorProbe = isMonitorProbeLog(record);
  const kind = getDetailKind(record, isQuotaAdjustment, isMonitorProbe);
  const meta = getDetailMeta(kind, t);
  const probeStatus = String(
    other.probe_status ||
      (record.content === '渠道探测成功' ? 'success' : 'failed'),
  ).toLowerCase();

  const isModelMapped =
    other.is_model_mapped &&
    other.upstream_model_name &&
    other.upstream_model_name !== '';

  const useTime = toNum(record.use_time);
  const frtMs = toNum(other.frt);
  const speed =
    useTime > 0 && toNum(record.completion_tokens) > 0
      ? Math.round(toNum(record.completion_tokens) / useTime)
      : 0;

  const cacheReadTokens = toNum(other.cache_tokens);
  const cacheWrite5m = toNum(other.cache_creation_tokens_5m);
  const cacheWrite1h = toNum(other.cache_creation_tokens_1h);
  const cacheWriteTotal =
    cacheWrite5m + cacheWrite1h > 0
      ? cacheWrite5m + cacheWrite1h
      : toNum(other.cache_creation_tokens);
  const inputTokens = toNum(record.prompt_tokens);
  const outputTokens = toNum(record.completion_tokens);
  const cacheHitRate =
    inputTokens > 0 && cacheReadTokens > 0
      ? Math.min(100, (cacheReadTokens / inputTokens) * 100)
      : 0;

  const ss = other.stream_status || null;
  const billingProcessRow = findRow('计费过程');
  const probe = isMonitorProbe
    ? {
        success: probeStatus === 'success',
        status: probeStatus,
        statusCode: other.status_code ?? null,
        errorCode: other.error_code || null,
        errorType: other.error_type || null,
        errorMessage:
          other.error_message ||
          (probeStatus === 'failed' ? record.content : null),
        billingScope: other.billing_scope || null,
      }
    : null;
  const error = kind === 'error' ? buildErrorDetail(record, other) : null;
  const createdAt =
    record.timestamp2string || formatTimestamp(record.created_at);
  const requestPath = other.request_path || null;
  const showUsage = ['consume', 'probe'].includes(kind);
  const showBilling = ['consume', 'probe'].includes(kind);
  const event = meta.eventTitle
    ? {
        title: meta.eventTitle,
        content: record.content || t('未记录'),
        rows: buildEventRows(kind, record, other, t),
      }
    : null;

  return {
    id: record.id,
    type: record.type,
    kind,
    typeLabel: meta.label,
    title: meta.title,
    createdAt,
    requestId: record.request_id || '',
    group: record.group || other.group || '-',
    tokenName: record.token_name || '-',
    modelName: record.model_name || '-',
    upstreamModelName: isModelMapped ? other.upstream_model_name : null,
    reasoningEffort: other.reasoning_effort || null,
    useTime,
    frtSeconds: frtMs > 0 ? Number((frtMs / 1000).toFixed(1)) : null,
    isStream: Boolean(record.is_stream),
    speed,
    streamStatus: ss,
    tokens: {
      input: inputTokens,
      output: outputTokens,
      cacheRead: cacheReadTokens,
      cacheWriteTotal,
      cacheWrite5m,
      cacheWrite1h,
      total: inputTokens + outputTokens,
      hitRate: cacheHitRate,
    },
    billing: buildBilling(record, other, t),
    billingProcess: billingProcessRow ? billingProcessRow.value : null,
    probe,
    error,
    event,
    showUsage,
    showBilling,
    billingTitle: kind === 'probe' ? t('标准口径参考') : t('计费详情'),
    usageTitle: kind === 'probe' ? t('探测用量') : t('消耗明细'),
    overviewItems: buildOverviewItems({
      kind,
      record,
      createdAt,
      requestId: record.request_id || '',
      group: record.group || other.group || '',
      tokenName: record.token_name || '',
      modelName: record.model_name || '',
      reasoningEffort: other.reasoning_effort || null,
      upstreamModelName: isModelMapped ? other.upstream_model_name : null,
      useTime,
      requestPath,
      t,
    }),
    violation: isViolationFeeLog(other)
      ? {
          feeText: renderQuota(other?.fee_quota ?? record?.quota ?? 0, 6),
        }
      : null,
    requestPath,
    nativeContent: findRow('日志详情')?.value || null,
    contentText: isQuotaAdjustment
      ? getAdminQuotaAdjustmentContent(record.content)
      : record.content || null,
    isAdminQuotaAdjustment: isQuotaAdjustment,
    extraRows,
  };
}

// 使用日志"详情"列的一行式简要摘要（参考新版 NewAPI："价格 · ¥2 / ¥12/M +1"），
// 点击摘要打开详情模态框；过多内容不再平铺在列表中。
export function buildUsageLogBriefSummary(record, t) {
  if (!record) {
    return null;
  }
  if (isMonitorProbeLog(record)) {
    const other = getLogOther(record.other) || {};
    const probeStatus = String(
      other.probe_status ||
        (record.content === '渠道探测成功' ? 'success' : 'failed'),
    ).toLowerCase();
    if (probeStatus === 'failed') {
      return `${t('探测失败')} · ${other.error_code || other.error_type || t('查看详情')}`;
    }
    return `${t('探测成功')} · ${formatCount(toNum(record.prompt_tokens) + toNum(record.completion_tokens))} Token`;
  }
  if (record.type === 5) {
    return t('错误详情');
  }
  if (record.type === 6) {
    return t('异步任务退款');
  }
  if (isAdminQuotaAdjustmentLog(record)) {
    return getAdminQuotaAdjustmentContent(record.content);
  }
  if (record.type !== 2) {
    return t('查看详情');
  }
  const other = getLogOther(record.other) || {};

  if (isViolationFeeLog(other)) {
    const feeQuota = other?.fee_quota ?? record?.quota ?? 0;
    return `${t('违规扣费')} · ${renderQuota(feeQuota, 6)}`;
  }
  if (other?.billing_source === 'subscription') {
    return t('订阅抵扣');
  }

  const modelPrice = other?.model_price;
  let modeLabel = null;
  let tierLabel = null;
  let inputPrice = null;
  let outputPrice = null;
  let extras = 0;

  if (other?.billing_mode === 'tiered_expr' && other?.expr_b64) {
    let exprStr = '';
    try {
      exprStr = decodeFromBase64(other.expr_b64);
    } catch (e) {
      exprStr = '';
    }
    const tiers = parseTiersFromExpr(exprStr);
    const tier =
      tiers.find((item) => item.label === other.matched_tier) || tiers[0];
    if (tier) {
      modeLabel = t('阶梯');
      tierLabel = tier.label;
      inputPrice = toNum(tier.inputPrice);
      outputPrice = toNum(tier.outputPrice);
      extras = TIERED_VAR_DEFS.filter(
        (def) =>
          ['cr', 'cc', 'cc1h'].includes(def.key) && toNum(tier[def.field]) > 0,
      ).length;
    }
  } else if (modelPrice != null && modelPrice !== -1) {
    modeLabel = t('按次');
    inputPrice = toNum(modelPrice);
  } else if (
    other &&
    (other.model_ratio != null || other.completion_ratio != null)
  ) {
    modeLabel = t('价格');
    const modelRatio = toNum(other.model_ratio);
    inputPrice = modelRatio * 2;
    outputPrice = modelRatio * 2 * toRatio(other.completion_ratio, 0);
    extras =
      (toNum(other.cache_tokens) > 0 ? 1 : 0) +
      (toNum(other.cache_creation_tokens) > 0 ||
      toNum(other.cache_creation_tokens_5m) > 0 ||
      toNum(other.cache_creation_tokens_1h) > 0
        ? 1
        : 0) +
      (other.image ? 1 : 0) +
      (other.ws || other.audio ? 1 : 0);
  }

  if (modeLabel == null) {
    return t('查看详情');
  }

  let text = tierLabel ? `${modeLabel}(${tierLabel})` : modeLabel;
  if (inputPrice != null) {
    text += ` · ${formatCompactPrice(inputPrice)}`;
    if (outputPrice != null) {
      text += ` / ${formatCompactPrice(outputPrice)}/M`;
    }
  }
  if (extras > 0) {
    text += ` +${extras}`;
  }
  return text;
}
