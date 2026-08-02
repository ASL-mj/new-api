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

export const MONITOR_TIMELINE_SIZE = 60;
export const MONITOR_STATUS_VIEW_KEY = 'monitor_status_view';

export const padMonitorTimeline = (timeline) => {
  const source = Array.isArray(timeline)
    ? timeline.slice(-MONITOR_TIMELINE_SIZE)
    : [];
  const padding = Array.from(
    { length: MONITOR_TIMELINE_SIZE - source.length },
    () => ({ status: 'empty', checked_at: null }),
  );
  return [...padding, ...source];
};

export const readMonitorStatusView = (storage = globalThis.localStorage) => {
  try {
    const value = storage?.getItem(MONITOR_STATUS_VIEW_KEY);
    return value === 'list' ? 'list' : 'card';
  } catch {
    return 'card';
  }
};

export const writeMonitorStatusView = (
  value,
  storage = globalThis.localStorage,
) => {
  if (value !== 'card' && value !== 'list') return;
  try {
    storage?.setItem(MONITOR_STATUS_VIEW_KEY, value);
  } catch {
    // The current view still works when storage is unavailable.
  }
};

export const monitorStatusMeta = {
  operational: { label: '正常', color: 'green' },
  degraded: { label: '降级', color: 'orange' },
  failed: { label: '故障', color: 'red' },
  timeout: { label: '超时', color: 'red' },
  unknown: { label: '暂无数据', color: 'grey' },
};

export const getMonitorStatusMeta = (status) =>
  monitorStatusMeta[status] || monitorStatusMeta.unknown;

export const formatMonitorAvailability = (group) => {
  const value = group?.availability ?? group?.availability_30d;
  return value == null ? '--' : `${Number(value).toFixed(2)}%`;
};

export const formatMonitorLatency = (value) =>
  value == null || Number(value) <= 0 ? '--' : `${Number(value)} ms`;

export const formatMonitorLatencyValue = (value) =>
  value == null || Number(value) <= 0 ? '--' : Number(value).toLocaleString();

export const getMonitorProviderIconName = (types = []) => {
  const type = String(types[0] || '').toLowerCase();
  if (
    type.includes('openai') ||
    type.includes('azure') ||
    type.includes('codex')
  ) {
    return 'OpenAI';
  }
  if (type.includes('anthropic') || type.includes('claude')) return 'Claude';
  if (
    type.includes('gemini') ||
    type.includes('google') ||
    type.includes('vertex')
  ) {
    return 'Gemini';
  }
  if (type.includes('deepseek')) return 'DeepSeek';
  if (type.includes('qwen') || type.includes('通义') || type.includes('阿里')) {
    return 'Qwen';
  }
  if (type.includes('mistral')) return 'Mistral';
  if (type.includes('grok') || type.includes('xai')) return 'XAI';
  if (type.includes('openrouter')) return 'OpenRouter';
  if (type.includes('ollama')) return 'Ollama';
  return 'Layers';
};

export const monitorTimelineHeight = (status) => {
  switch (status) {
    case 'operational':
      return 100;
    case 'degraded':
      return 65;
    case 'failed':
    case 'timeout':
      return 35;
    default:
      return 15;
  }
};

export const formatMonitorRefresh = (seconds, t) => {
  const value = Math.max(0, Number(seconds) || 0);
  return t('{{seconds}} s 后刷新', { seconds: value });
};
