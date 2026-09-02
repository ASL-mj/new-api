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

import { normalizeLucideIconName } from './lucide';

const CUSTOM_PLACEMENTS = new Set([
  'topbar',
  'chat',
  'console',
  'personal',
  'admin',
]);

export const USER_CONTROLLED_CUSTOM_PLACEMENTS = new Set([
  'chat',
  'console',
  'personal',
]);

export const normalizeSidebarCustomItems = (items) => {
  if (!Array.isArray(items)) return [];
  return items.map((item, index) => ({
    id: String(item?.id || `custom-${index + 1}`),
    name: String(item?.name || '').trim(),
    description: String(item?.description || '').trim(),
    url: String(item?.url || '').trim(),
    icon: normalizeLucideIconName(item?.icon),
    enabled: item?.enabled !== false,
    // Keep menus created by the first implementation visible, but place them
    // in an existing sidebar section instead of creating a fifth section.
    placement:
      item?.placement === 'sidebar'
        ? 'console'
        : CUSTOM_PLACEMENTS.has(item?.placement)
          ? item.placement
          : 'console',
    openMode: 'iframe',
  }));
};

export const isSafeSidebarUrl = (value) => {
  try {
    const url = new URL(String(value || '').trim());
    return (
      (url.protocol === 'http:' || url.protocol === 'https:') && !!url.host
    );
  } catch {
    return false;
  }
};

export const isCustomMenuVisibleForUser = (item, userCustomConfig = {}) =>
  item?.enabled !== false &&
  USER_CONTROLLED_CUSTOM_PLACEMENTS.has(item?.placement) &&
  userCustomConfig?.[item.id] !== false;

export const createDefaultUserConfig = (adminConfig) => {
  const defaultUserConfig = {};

  Object.keys(adminConfig || {}).forEach((sectionKey) => {
    if (sectionKey === 'custom') return;

    const sectionConfig = adminConfig[sectionKey];
    if (!sectionConfig?.enabled) return;

    defaultUserConfig[sectionKey] = { enabled: true };
    Object.keys(sectionConfig).forEach((moduleKey) => {
      if (moduleKey !== 'enabled' && sectionConfig[moduleKey]) {
        defaultUserConfig[sectionKey][moduleKey] = true;
      }
    });
  });

  defaultUserConfig.custom = {};
  normalizeSidebarCustomItems(adminConfig?.custom).forEach((item) => {
    if (isCustomMenuVisibleForUser(item)) {
      defaultUserConfig.custom[item.id] = true;
    }
  });

  return defaultUserConfig;
};

export const getCustomExternalMenuItems = (
  config,
  placement,
  userCustomConfig = null,
) =>
  normalizeSidebarCustomItems(config?.custom)
    .filter(
      (item) =>
        item.enabled &&
        item.placement === placement &&
        item.name &&
        isSafeSidebarUrl(item.url) &&
        (userCustomConfig === null ||
          placement === 'topbar' ||
          placement === 'admin' ||
          isCustomMenuVisibleForUser(item, userCustomConfig)),
    )
    .map((item) => ({
      text: item.name,
      itemKey: `custom:${item.id}`,
      iconKey: item.icon,
      to:
        placement === 'topbar'
          ? `/external/${encodeURIComponent(item.id)}`
          : `/console/external/${encodeURIComponent(item.id)}`,
      requireAuth: true,
    }));
