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
  getCustomExternalMenuItems,
  isCustomMenuVisibleForUser,
  normalizeSidebarCustomItems,
  createDefaultUserConfig,
} from '../../helpers/externalMenu';

describe('custom external menu placement', () => {
  test('renders only enabled safe links at their configured placement', () => {
    const config = {
      custom: [
        {
          id: 'topbar-link',
          name: 'Status',
          url: 'https://status.example.com',
          placement: 'topbar',
        },
        {
          id: 'disabled-link',
          name: 'Disabled',
          url: 'https://disabled.example.com',
          placement: 'topbar',
          enabled: false,
        },
        {
          id: 'unsafe-link',
          name: 'Unsafe',
          url: 'javascript:alert(1)',
          placement: 'topbar',
        },
      ],
    };

    expect(getCustomExternalMenuItems(config, 'topbar')).toEqual([
      {
        text: 'Status',
        itemKey: 'custom:topbar-link',
        to: '/external/topbar-link',
        iconKey: 'external',
        requireAuth: true,
      },
    ]);
  });

  test('moves legacy generic sidebar items into the console section', () => {
    expect(
      normalizeSidebarCustomItems([
        {
          id: 'legacy-link',
          name: 'Legacy',
          url: 'https://legacy.example.com',
          placement: 'sidebar',
        },
      ]),
    ).toMatchObject([{ placement: 'console' }]);
  });

  test('normalizes optional menu descriptions without exposing the source URL', () => {
    expect(
      normalizeSidebarCustomItems([
        {
          id: 'document-link',
          name: 'Documentation',
          description: 'Integration guides',
          url: 'https://docs.example.com',
          placement: 'console',
        },
        {
          id: 'legacy-link',
          name: 'Legacy',
          url: 'https://legacy.example.com',
          placement: 'console',
        },
      ]),
    ).toMatchObject([
      { description: 'Integration guides' },
      { description: '' },
    ]);
  });

  test('uses a console route for menus embedded in sidebar sections', () => {
    expect(
      getCustomExternalMenuItems(
        {
          custom: [
            {
              id: 'console-link',
              name: 'Console link',
              url: 'https://console.example.com',
              placement: 'console',
            },
          ],
        },
        'console',
      ),
    ).toMatchObject([{ to: '/console/external/console-link' }]);
  });

  test('allows users to hide sidebar menus without affecting topbar menus', () => {
    const config = {
      custom: [
        {
          id: 'console-link',
          name: 'Console link',
          url: 'https://console.example.com',
          placement: 'console',
        },
        {
          id: 'topbar-link',
          name: 'Topbar link',
          url: 'https://topbar.example.com',
          placement: 'topbar',
        },
      ],
    };

    expect(
      getCustomExternalMenuItems(config, 'console', { 'console-link': false }),
    ).toEqual([]);
    expect(
      getCustomExternalMenuItems(config, 'topbar', { 'topbar-link': false }),
    ).toHaveLength(1);
    expect(
      isCustomMenuVisibleForUser(
        { id: 'console-link', placement: 'console' },
        {},
      ),
    ).toBe(true);
    expect(
      isCustomMenuVisibleForUser(
        { id: 'console-link', placement: 'console' },
        { 'console-link': false },
      ),
    ).toBe(false);
    expect(
      isCustomMenuVisibleForUser({ id: 'admin-link', placement: 'admin' }, {}),
    ).toBe(false);
  });

  test('includes enabled custom sidebar menus in a new user default', () => {
    const config = {
      console: { enabled: true, log: true },
      custom: [
        {
          id: 'console-link',
          name: 'Console link',
          url: 'https://console.example.com',
          placement: 'console',
        },
        {
          id: 'topbar-link',
          name: 'Topbar link',
          url: 'https://topbar.example.com',
          placement: 'topbar',
        },
      ],
    };

    expect(createDefaultUserConfig(config).custom).toEqual({
      'console-link': true,
    });
  });
});
