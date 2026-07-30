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

const CACHE_TTL_MS = 60 * 1000;
const performanceCache = new Map();
const inFlightRequests = new Map();

function getCacheKey(modelName, hours) {
  return `${modelName}:${hours}`;
}

function isFresh(entry) {
  return entry && Date.now() - entry.fetchedAt < CACHE_TTL_MS;
}

function normalizeError(error) {
  const message =
    error?.response?.data?.message ||
    error?.message ||
    'Failed to load performance data';
  return new Error(message);
}

function loadPerformanceData(key, modelName, hours) {
  const pendingRequest = inFlightRequests.get(key);
  if (pendingRequest) return pendingRequest;

  const request = API.get('/api/perf-metrics', {
    params: { model: modelName, hours },
    disableDuplicate: true,
    skipErrorHandler: true,
  })
    .then((response) => {
      if (!response.data?.success || !response.data?.data) {
        throw new Error(
          response.data?.message || 'Failed to load performance data',
        );
      }

      const data = response.data.data;
      performanceCache.set(key, { data, fetchedAt: Date.now() });
      return data;
    })
    .finally(() => {
      if (inFlightRequests.get(key) === request) {
        inFlightRequests.delete(key);
      }
    });
  inFlightRequests.set(key, request);
  return request;
}

export function useModelPerformance({ modelName, hours, enabled }) {
  const [state, setState] = useState({
    key: '',
    status: 'idle',
    data: null,
    error: null,
  });
  const [requestVersion, setRequestVersion] = useState(0);
  const forcedRefreshKeyRef = useRef(null);

  useEffect(() => {
    if (!enabled || !modelName) {
      setState({ key: '', status: 'idle', data: null, error: null });
      return undefined;
    }

    const key = getCacheKey(modelName, hours);
    const cached = performanceCache.get(key);
    const forceRefresh = forcedRefreshKeyRef.current === key;
    let cancelled = false;
    let lastGoodData = cached?.data || null;

    if (isFresh(cached) && !forceRefresh) {
      setState({ key, status: 'success', data: cached.data, error: null });
      return undefined;
    }

    if (forceRefresh) {
      forcedRefreshKeyRef.current = null;
    }

    setState((current) => {
      lastGoodData = current.key === key ? current.data : cached?.data || null;
      return {
        key,
        status: 'loading',
        data: lastGoodData,
        error: null,
      };
    });

    loadPerformanceData(key, modelName, hours)
      .then((data) => {
        if (cancelled) return;
        setState({ key, status: 'success', data, error: null });
      })
      .catch((error) => {
        if (cancelled) return;
        setState({
          key,
          status: 'error',
          data: lastGoodData,
          error: normalizeError(error),
        });
      });

    return () => {
      cancelled = true;
    };
  }, [modelName, hours, enabled, requestVersion]);

  const retry = () => {
    if (!modelName || state.status === 'loading') return;
    forcedRefreshKeyRef.current = getCacheKey(modelName, hours);
    setRequestVersion((version) => version + 1);
  };

  return { ...state, retry };
}
