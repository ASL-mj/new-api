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

import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { API, showError } from '../../helpers';
import {
  readMonitorStatusView,
  writeMonitorStatusView,
} from '../../components/monitor-status/monitorTimelineUtils';

const AUTO_REFRESH_SECONDS = 60;

export const useMonitorStatusData = () => {
  const { t } = useTranslation();
  const [days, setDays] = useState('30');
  const [view, setViewState] = useState(() => readMonitorStatusView());
  const [groups, setGroups] = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshAfter, setRefreshAfter] = useState(AUTO_REFRESH_SECONDS);
  const autoRefreshPendingRef = useRef(false);
  const [selected, setSelected] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const loadStatus = useCallback(
    async ({ silent = false } = {}) => {
      if (!silent) setLoading(true);
      try {
        const response = await API.get('/api/monitor_status/', {
          params: { days },
        });
        if (!response.data?.success) {
          showError(response.data?.message || t('分组状态加载失败'));
          return false;
        }
        setGroups(response.data.data || []);
        return true;
      } catch (error) {
        if (!silent) {
          showError(error.response?.data?.message || t('分组状态加载失败'));
        }
        return false;
      } finally {
        if (!silent) setLoading(false);
      }
    },
    [days, t],
  );

  const refresh = async () => {
    setRefreshing(true);
    setRefreshAfter(AUTO_REFRESH_SECONDS);
    await loadStatus({ silent: true });
    setRefreshing(false);
  };

  const setView = (value) => {
    setViewState(value);
    writeMonitorStatusView(value);
  };

  const openDetails = async (group) => {
    setSelected(group);
    setDetailLoading(true);
    try {
      const response = await API.get(`/api/monitor_status/${group.id}`, {
        params: { days },
      });
      if (response.data?.success) {
        setSelected(response.data.data);
      } else {
        showError(response.data?.message || t('分组详情加载失败'));
      }
    } catch (error) {
      showError(error.response?.data?.message || t('分组详情加载失败'));
    } finally {
      setDetailLoading(false);
    }
  };

  const closeDetails = () => setSelected(null);

  useEffect(() => {
    setSelected(null);
    setRefreshAfter(AUTO_REFRESH_SECONDS);
    loadStatus();
  }, [loadStatus]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      setRefreshAfter((current) => {
        if (current <= 1) {
          if (!autoRefreshPendingRef.current) {
            autoRefreshPendingRef.current = true;
            loadStatus({ silent: true }).finally(() => {
              autoRefreshPendingRef.current = false;
            });
          }
          return AUTO_REFRESH_SECONDS;
        }
        return current - 1;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [loadStatus]);

  return {
    t,
    days,
    setDays,
    view,
    setView,
    groups,
    loading,
    refreshing,
    refreshAfter,
    refresh,
    selected,
    detailLoading,
    openDetails,
    closeDetails,
  };
};
