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

const DASH = '-';

function isPositiveFinite(value) {
  return Number.isFinite(value) && value > 0;
}

function formatNumber(value, options) {
  return new Intl.NumberFormat(undefined, options).format(value);
}

export function formatRequestCount(value) {
  if (!Number.isFinite(value) || value < 0) return DASH;
  return formatNumber(value, { maximumFractionDigits: 0 });
}

export function formatTps(value) {
  if (!isPositiveFinite(value)) return DASH;
  return `${formatNumber(value, { maximumFractionDigits: 2 })} TPS`;
}

export function formatLatency(value) {
  if (!isPositiveFinite(value)) return DASH;
  if (value < 1000) {
    return `${formatNumber(value, { maximumFractionDigits: 0 })} ms`;
  }
  return `${formatNumber(value / 1000, { maximumFractionDigits: 2 })} s`;
}

export function formatPercentage(value) {
  if (!Number.isFinite(value)) return DASH;
  const clamped = Math.min(100, Math.max(0, value));
  return `${formatNumber(clamped, { maximumFractionDigits: 2 })}%`;
}

export function formatChartTime(timestamp, hours) {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return DASH;
  const date = new Date(timestamp * 1000);
  const options =
    hours === 1
      ? { hour: '2-digit', minute: '2-digit', hour12: false }
      : {
          month: '2-digit',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        };
  return new Intl.DateTimeFormat(undefined, options).format(date);
}
