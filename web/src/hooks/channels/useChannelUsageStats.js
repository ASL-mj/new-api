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
