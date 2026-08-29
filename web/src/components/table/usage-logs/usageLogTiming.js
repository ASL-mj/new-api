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

const DURATION_TONE_THRESHOLDS = Object.freeze({
  warning: 101,
  danger: 300,
});

const FIRST_RESPONSE_TONE_THRESHOLDS = Object.freeze({
  warning: 3000,
  danger: 10000,
});

export function getDurationTone(value) {
  const seconds = Number(value);
  if (!Number.isFinite(seconds) || seconds < DURATION_TONE_THRESHOLDS.warning) {
    return 'success';
  }
  if (seconds < DURATION_TONE_THRESHOLDS.danger) {
    return 'warning';
  }
  return 'danger';
}

export function getFirstResponseTone(value) {
  const milliseconds = Number(value);
  if (
    !Number.isFinite(milliseconds) ||
    milliseconds < FIRST_RESPONSE_TONE_THRESHOLDS.warning
  ) {
    return 'success';
  }
  if (milliseconds < FIRST_RESPONSE_TONE_THRESHOLDS.danger) {
    return 'warning';
  }
  return 'danger';
}

export function getTimingToneStyles(tone) {
  const resolvedTone = ['success', 'warning', 'danger'].includes(tone)
    ? tone
    : 'success';
  return {
    color: `var(--semi-color-${resolvedTone})`,
    backgroundColor: `var(--semi-color-${resolvedTone}-light-default)`,
    borderColor: `var(--semi-color-${resolvedTone}-light-hover)`,
  };
}
