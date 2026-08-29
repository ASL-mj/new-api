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

import { useEffect, useRef, useState } from 'react';
import { API } from '../../helpers';

const collectChannelIds = (channels) => {
  const ids = [];
  channels.forEach((record) => {
    const rows = Array.isArray(record.children) ? record.children : [record];
    rows.forEach((row) => {
      const id = Number(row.id);
      if (Number.isInteger(id) && id > 0) ids.push(id);
    });
  });
  return [...new Set(ids)];
};

export const useChannelUsageStats = (channels, enabled) => {
  const [usageStats, setUsageStats] = useState({});
  const [usageStatsLoading, setUsageStatsLoading] = useState(false);
  const [usageStatsError, setUsageStatsError] = useState('');
  const requestSequence = useRef(0);

  useEffect(() => {
    if (!enabled) return;
    const ids = collectChannelIds(channels);
    if (ids.length === 0) {
      setUsageStats({});
      return;
    }

    const sequence = ++requestSequence.current;
    setUsageStatsLoading(true);
    setUsageStatsError('');
    API.get('/api/channel/usage/batch', { params: { ids: ids.join(',') } })
      .then((response) => {
        if (sequence !== requestSequence.current) return;
        if (!response.data.success) throw new Error(response.data.message);
        setUsageStats(response.data.data || {});
      })
      .catch((error) => {
        if (sequence !== requestSequence.current) return;
        setUsageStatsError(error?.message || 'load failed');
      })
      .finally(() => {
        if (sequence === requestSequence.current) setUsageStatsLoading(false);
      });
  }, [channels, enabled]);

  return { usageStats, usageStatsLoading, usageStatsError };
};
