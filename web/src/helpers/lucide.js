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

import { iconNames } from 'lucide-react/dynamic';

const LUCIDE_ICON_NAMES = [...iconNames].sort();
const LUCIDE_ICON_NAME_SET = new Set(LUCIDE_ICON_NAMES);

const toKebabCase = (value) =>
  String(value || '')
    .trim()
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/([A-Z])([A-Z][a-z])/g, '$1-$2')
    .replace(/[\s_]+/g, '-')
    .toLowerCase();

export const normalizeLucideIconName = (value) => {
  const iconName = toKebabCase(value);
  return LUCIDE_ICON_NAME_SET.has(iconName) ? iconName : 'external-link';
};

export const getLucideIconNames = () => LUCIDE_ICON_NAMES;
