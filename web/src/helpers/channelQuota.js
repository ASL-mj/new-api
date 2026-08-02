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

import {
  displayAmountToQuota as convertAmountToQuota,
  quotaToDisplayAmount as convertQuotaToAmount,
} from './quota';

const parseLimitValue = (value) => {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
};

export const quotaToDisplayAmount = (quota) => {
  const normalizedQuota = parseLimitValue(quota);
  return normalizedQuota === 0 ? 0 : convertQuotaToAmount(normalizedQuota);
};

export const displayAmountToQuota = (amount) => {
  const normalizedAmount = parseLimitValue(amount);
  return normalizedAmount === 0
    ? 0
    : Math.max(0, Math.round(convertAmountToQuota(normalizedAmount)));
};
