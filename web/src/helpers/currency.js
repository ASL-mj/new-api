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

export function getCurrencyConfig() {
  const quotaDisplayType = localStorage.getItem('quota_display_type') || 'USD';
  const statusStr = localStorage.getItem('status');

  let symbol = '$';
  let rate = 1;

  if (quotaDisplayType === 'CNY') {
    symbol = '¥';
    try {
      if (statusStr) {
        const status = JSON.parse(statusStr);
        rate = status?.usd_exchange_rate || 7;
      }
    } catch (error) {}
  } else if (quotaDisplayType === 'CUSTOM') {
    try {
      if (statusStr) {
        const status = JSON.parse(statusStr);
        symbol = status?.custom_currency_symbol || '¤';
        rate = status?.custom_currency_exchange_rate || 1;
      }
    } catch (error) {}
  }

  return { symbol, rate, type: quotaDisplayType };
}
