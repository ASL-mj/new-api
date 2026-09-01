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

export const OPS_PERIOD_OPTIONS = [
  { value: '1h', label: '近 1 小时', seconds: 60 * 60 },
  { value: '6h', label: '近 6 小时', seconds: 6 * 60 * 60 },
  { value: '24h', label: '近 24 小时', seconds: 24 * 60 * 60 },
  { value: '7d', label: '近 7 天', seconds: 7 * 24 * 60 * 60 },
  { value: '30d', label: '近 30 天', seconds: 30 * 24 * 60 * 60 },
];

const DEFAULT_FILTERS = {
  period: '1h',
  channelType: undefined,
  group: '',
  model: '',
};

export const toOpsParams = (filters) => {
  const now = Math.floor(Date.now() / 1000);
  const period =
    OPS_PERIOD_OPTIONS.find((item) => item.value === filters.period) ||
    OPS_PERIOD_OPTIONS[0];
  const params = {
    start_timestamp: now - period.seconds,
    end_timestamp: now,
  };
  if (filters.channelType !== undefined && filters.channelType !== null) {
    params.channel_type = filters.channelType;
  }
  if (filters.group?.trim()) params.group = filters.group.trim();
  if (filters.model?.trim()) params.model = filters.model.trim();
  return params;
};

export const formatOpsTimestamp = (timestamp) => {
  if (!timestamp) return '--';
  return new Date(timestamp * 1000).toLocaleString();
};

export const useOpsData = () => {
  const { t } = useTranslation();
  const [draftFilters, setDraftFilters] = useState(DEFAULT_FILTERS);
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  const [overview, setOverview] = useState(null);
  const [system, setSystem] = useState(null);
  const [rankings, setRankings] = useState([]);
  const [overviewLoading, setOverviewLoading] = useState(true);
  const [systemLoading, setSystemLoading] = useState(true);
  const [rankingsLoading, setRankingsLoading] = useState(true);
  const [overviewError, setOverviewError] = useState('');
  const [systemError, setSystemError] = useState('');
  const [rankingsError, setRankingsError] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [waveWindow, setWaveWindow] = useState('1h');

  const [detailMetric, setDetailMetric] = useState(null);
  const [detailData, setDetailData] = useState({ items: [], total: 0 });
  const [detailPage, setDetailPage] = useState(1);
  const [detailsLoading, setDetailsLoading] = useState(false);
  const [detailRequestId, setDetailRequestId] = useState('');
  const [eventDetail, setEventDetail] = useState(null);

  const [logDraft, setLogDraft] = useState({
    level: '',
    component: '',
    requestId: '',
  });
  const [logFilters, setLogFilters] = useState(logDraft);
  const [logPage, setLogPage] = useState(1);
  const [logs, setLogs] = useState({ items: [], total: 0 });
  const [logsLoading, setLogsLoading] = useState(true);
  const [logsError, setLogsError] = useState('');

  const sequenceRef = useRef({
    overview: 0,
    system: 0,
    rankings: 0,
    details: 0,
    logs: 0,
  });

  const loadOverview = useCallback(
    async ({ silent = false } = {}) => {
      const sequence = ++sequenceRef.current.overview;
      if (!silent) setOverviewLoading(true);
      setOverviewError('');
      try {
        const response = await API.get('/api/ops/overview', {
          params: toOpsParams(filters),
        });
        if (sequence !== sequenceRef.current.overview) return false;
        if (!response.data?.success) {
          setOverviewError(response.data?.message || t('运维概览加载失败'));
          return false;
        }
        setOverview(response.data.data || null);
        return true;
      } catch (error) {
        if (sequence === sequenceRef.current.overview) {
          setOverviewError(
            error.response?.data?.message || t('运维概览加载失败'),
          );
        }
        return false;
      } finally {
        if (!silent && sequence === sequenceRef.current.overview) {
          setOverviewLoading(false);
        }
      }
    },
    [filters, t],
  );

  const loadSystem = useCallback(
    async ({ silent = false } = {}) => {
      const sequence = ++sequenceRef.current.system;
      if (!silent) setSystemLoading(true);
      setSystemError('');
      try {
        const response = await API.get('/api/ops/system');
        if (sequence !== sequenceRef.current.system) return false;
        if (!response.data?.success) {
          setSystemError(response.data?.message || t('服务器状态加载失败'));
          return false;
        }
        setSystem(response.data.data || null);
        return true;
      } catch (error) {
        if (sequence === sequenceRef.current.system) {
          setSystemError(
            error.response?.data?.message || t('服务器状态加载失败'),
          );
        }
        return false;
      } finally {
        if (!silent && sequence === sequenceRef.current.system) {
          setSystemLoading(false);
        }
      }
    },
    [t],
  );

  const loadRankings = useCallback(
    async ({ silent = false } = {}) => {
      const sequence = ++sequenceRef.current.rankings;
      if (!silent) setRankingsLoading(true);
      setRankingsError('');
      try {
        const response = await API.get('/api/ops/rankings', {
          params: toOpsParams(filters),
        });
        if (sequence !== sequenceRef.current.rankings) return false;
        if (!response.data?.success) {
          setRankingsError(response.data?.message || t('排行加载失败'));
          return false;
        }
        setRankings(response.data.data || []);
        return true;
      } catch (error) {
        if (sequence === sequenceRef.current.rankings) {
          setRankingsError(error.response?.data?.message || t('排行加载失败'));
        }
        return false;
      } finally {
        if (!silent && sequence === sequenceRef.current.rankings) {
          setRankingsLoading(false);
        }
      }
    },
    [filters, t],
  );

  const loadLogs = useCallback(
    async ({ silent = false, page = 1 } = {}) => {
      const sequence = ++sequenceRef.current.logs;
      if (!silent) setLogsLoading(true);
      setLogsError('');
      try {
        const response = await API.get('/api/system_event_log/', {
          params: {
            p: page,
            page_size: 10,
            level: logFilters.level || undefined,
            component: logFilters.component.trim() || undefined,
            request_id: logFilters.requestId.trim() || undefined,
          },
        });
        if (sequence !== sequenceRef.current.logs) return false;
        if (!response.data?.success) {
          setLogsError(response.data?.message || t('系统运行日志加载失败'));
          return false;
        }
        const data = response.data.data || {};
        setLogs({ items: data.items || [], total: data.total || 0 });
        setLogPage(data.page || page);
        return true;
      } catch (error) {
        if (sequence === sequenceRef.current.logs) {
          setLogsError(
            error.response?.data?.message || t('系统运行日志加载失败'),
          );
        }
        return false;
      } finally {
        if (!silent && sequence === sequenceRef.current.logs) {
          setLogsLoading(false);
        }
      }
    },
    [logFilters, t],
  );

  const loadDetails = useCallback(
    async (metric, page = 1) => {
      const sequence = ++sequenceRef.current.details;
      setDetailsLoading(true);
      try {
        const detailFilters = detailRequestId ? {} : toOpsParams(filters);
        const response = await API.get('/api/ops/details', {
          params: {
            ...detailFilters,
            metric: metric.key,
            request_id: detailRequestId || undefined,
            p: page,
            page_size: 20,
          },
        });
        if (sequence !== sequenceRef.current.details) return;
        if (!response.data?.success) {
          showError(response.data?.message || t('指标明细加载失败'));
          return;
        }
        const pageData = response.data.data?.page || {};
        setDetailData({
          items: pageData.items || [],
          total: pageData.total || 0,
        });
        setDetailPage(pageData.page || page);
      } catch (error) {
        if (sequence === sequenceRef.current.details) {
          showError(error.response?.data?.message || t('指标明细加载失败'));
        }
      } finally {
        if (sequence === sequenceRef.current.details) setDetailsLoading(false);
      }
    },
    [detailRequestId, filters, t],
  );

  const openDetail = (metric, requestId = '') => {
    sequenceRef.current.details++;
    setEventDetail(null);
    setDetailMetric(metric);
    setDetailRequestId(requestId);
    setDetailData({ items: [], total: 0 });
    setDetailPage(1);
  };

  const closeDetail = () => {
    sequenceRef.current.details++;
    setDetailMetric(null);
    setDetailRequestId('');
    setEventDetail(null);
    setDetailData({ items: [], total: 0 });
    setDetailPage(1);
  };

  const openEventDetail = (event, { showEvent = false } = {}) => {
    if (!event) return;
    if (event.request_id && !showEvent) {
      openDetail({ key: 'requests', title: t('请求详情') }, event.request_id);
      return;
    }
    sequenceRef.current.details++;
    setDetailMetric(null);
    setDetailRequestId('');
    setDetailData({ items: [], total: 0 });
    setDetailPage(1);
    setEventDetail(event);
  };

  const handleDetailPageChange = (page) => {
    setDetailPage(page);
    if (detailMetric) loadDetails(detailMetric, page);
  };

  const applyFilters = () => setFilters({ ...draftFilters });
  const resetFilters = () => {
    setDraftFilters(DEFAULT_FILTERS);
    setFilters(DEFAULT_FILTERS);
  };

  const applyLogFilters = () => {
    setLogPage(1);
    setLogFilters({ ...logDraft });
  };

  const resetLogFilters = () => {
    const empty = { level: '', component: '', requestId: '' };
    setLogDraft(empty);
    setLogPage(1);
    setLogFilters(empty);
  };

  const handleLogPageChange = (page) => {
    setLogPage(page);
  };

  const refresh = async () => {
    setRefreshing(true);
    await Promise.all([
      loadOverview({ silent: true }),
      loadSystem({ silent: true }),
      loadRankings({ silent: true }),
      loadLogs({ silent: true, page: logPage }),
    ]);
    setRefreshing(false);
  };

  useEffect(() => {
    loadOverview();
    loadRankings();
  }, [loadOverview, loadRankings]);

  useEffect(() => {
    loadSystem();
  }, [loadSystem]);

  useEffect(() => {
    loadLogs({ page: logPage });
  }, [loadLogs, logPage]);

  useEffect(() => {
    if (!detailMetric) return;
    setDetailPage(1);
    loadDetails(detailMetric, 1);
  }, [detailMetric, loadDetails]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState !== 'visible') return;
      loadOverview({ silent: true });
      loadSystem({ silent: true });
      loadRankings({ silent: true });
    }, 30000);
    return () => window.clearInterval(timer);
  }, [loadOverview, loadSystem, loadRankings]);

  useEffect(
    () => () => {
      Object.keys(sequenceRef.current).forEach((key) => {
        sequenceRef.current[key]++;
      });
    },
    [],
  );

  return {
    t,
    draftFilters,
    setDraftFilters,
    filters,
    applyFilters,
    resetFilters,
    overview,
    system,
    rankings,
    overviewLoading,
    systemLoading,
    rankingsLoading,
    overviewError,
    systemError,
    rankingsError,
    refreshing,
    refresh,
    waveWindow,
    setWaveWindow,
    detailMetric,
    detailData,
    detailPage,
    detailsLoading,
    detailRequestId,
    eventDetail,
    openDetail,
    openEventDetail,
    closeDetail,
    handleDetailPageChange,
    logDraft,
    setLogDraft,
    logs,
    logPage,
    logsLoading,
    logsError,
    applyLogFilters,
    resetLogFilters,
    handleLogPageChange,
  };
};
