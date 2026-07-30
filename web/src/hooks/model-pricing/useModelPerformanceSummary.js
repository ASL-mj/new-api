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

import { useEffect, useState } from 'react';
import { API } from '../../helpers';
import { createModelPerformanceSummaryStore } from './modelPerformanceSummaryStore';

const EMPTY_MAP = new Map();

const summaryStore = createModelPerformanceSummaryStore({
  fetchSummary: async (hours) => {
    const response = await API.get('/api/perf-metrics/summary', {
      params: { hours },
      disableDuplicate: true,
      skipErrorHandler: true,
    });
    if (!response.data?.success || !response.data?.data) {
      throw new Error(
        response.data?.message || 'Failed to load performance summary',
      );
    }
    return response.data.data;
  },
});

export function useModelPerformanceSummary({
  hours = 24,
  enabled = true,
} = {}) {
  const [models, setModels] = useState(EMPTY_MAP);

  useEffect(() => {
    if (!enabled) {
      setModels(EMPTY_MAP);
      return undefined;
    }

    let active = true;
    summaryStore
      .load(hours)
      .then((nextModels) => {
        if (active) {
          setModels(nextModels);
        }
      })
      .catch(() => {
        if (active) {
          setModels(EMPTY_MAP);
        }
      });

    return () => {
      active = false;
    };
  }, [enabled, hours]);

  return models;
}
