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
import { useTranslation } from 'react-i18next';

import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';
import { normalizeMonitorGroupPayload } from './monitorGroupUtils';

export {
  getCommonChannelModels,
  normalizeMonitorGroupPayload,
  parseChannelModels,
} from './monitorGroupUtils';

const parseExtraModels = (value) => {
  if (Array.isArray(value)) return value;
  if (!value) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};

export const getMonitorGroupFormValues = (group) => ({
  name: group?.name || '',
  key: group?.key || '',
  description: group?.description || '',
  primary_model: group?.primary_model || '',
  extra_models: parseExtraModels(group?.extra_models),
  channel_ids: (group?.targets || []).map((target) => target.channel_id),
  enabled: group?.enabled ?? true,
  user_visible: group?.user_visible ?? true,
  interval_seconds: group?.interval_seconds || 300,
  timeout_seconds: group?.timeout_seconds || 30,
  degraded_ms: group?.degraded_ms || 3000,
});

export const useMonitorGroupsData = () => {
  const { t } = useTranslation();
  const [groups, setGroups] = useState([]);
  const [channels, setChannels] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [saving, setSaving] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [total, setTotal] = useState(0);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [compactMode, setCompactMode] = useTableCompactMode('monitorGroups');
  const [showEdit, setShowEdit] = useState(false);
  const [editingGroup, setEditingGroup] = useState(null);
  const pollingRef = useRef(new Map());

  const syncPage = (page) => {
    setGroups(page?.items || []);
    setTotal(page?.total || 0);
    setActivePage(page?.page || 1);
    setPageSize(page?.page_size || pageSize);
  };

  const loadGroups = async ({
    page = activePage,
    size = pageSize,
    search = searchKeyword,
    silent = false,
  } = {}) => {
    if (!silent) setLoading(true);
    try {
      const response = await API.get('/api/monitor_group/', {
        params: {
          p: page,
          page_size: size,
          search: search.trim() || undefined,
        },
      });
      if (!response.data?.success) {
        showError(response.data?.message || t('渠道状态加载失败'));
        return false;
      }
      syncPage(response.data.data || {});
      return true;
    } catch (error) {
      if (!silent) {
        showError(error.response?.data?.message || t('渠道状态加载失败'));
      }
      return false;
    } finally {
      if (!silent) setLoading(false);
    }
  };

  const loadChannels = async () => {
    try {
      const response = await API.get('/api/monitor_group/channels');
      if (response.data?.success) {
        setChannels(response.data.data || []);
      } else {
        showError(response.data?.message || t('渠道列表加载失败'));
      }
    } catch (error) {
      showError(error.response?.data?.message || t('渠道列表加载失败'));
    }
  };

  const stopPolling = (groupId) => {
    const poll = pollingRef.current.get(groupId);
    if (poll) window.clearTimeout(poll.timer);
    pollingRef.current.delete(groupId);
  };

  const pollGroup = (groupId, timeoutSeconds) => {
    stopPolling(groupId);
    const deadline = Date.now() + (Number(timeoutSeconds) + 10) * 1000;

    const poll = async () => {
      try {
        const response = await API.get(`/api/monitor_group/${groupId}`);
        const group = response.data?.data;
        if (response.data?.success && group) {
          setGroups((current) =>
            current.map((item) => (item.id === groupId ? group : item)),
          );
          if (!group.running) {
            stopPolling(groupId);
            return;
          }
        }
      } catch {
        // A later poll can recover from a transient admin API failure.
      }

      if (Date.now() >= deadline) {
        stopPolling(groupId);
        await loadGroups({ silent: true });
        return;
      }
      const timer = window.setTimeout(poll, 1000);
      pollingRef.current.set(groupId, { timer });
    };

    const timer = window.setTimeout(poll, 400);
    pollingRef.current.set(groupId, { timer });
  };

  const runGroup = async (group) => {
    if (group.running) return;
    setGroups((current) =>
      current.map((item) =>
        item.id === group.id ? { ...item, running: true } : item,
      ),
    );
    try {
      const response = await API.post(`/api/monitor_group/${group.id}/run`);
      if (!response.data?.success) {
        throw new Error(response.data?.message || t('探测启动失败'));
      }
      showSuccess(t('探测任务已启动'));
      pollGroup(group.id, group.timeout_seconds || 30);
    } catch (error) {
      setGroups((current) =>
        current.map((item) =>
          item.id === group.id ? { ...item, running: false } : item,
        ),
      );
      showError(
        error.response?.data?.message || error.message || t('探测启动失败'),
      );
    }
  };

  const saveGroup = async (values) => {
    const payload = normalizeMonitorGroupPayload(values, editingGroup);
    setSaving(true);
    try {
      const response = payload.id
        ? await API.put('/api/monitor_group/', payload)
        : await API.post('/api/monitor_group/', payload);
      if (!response.data?.success) {
        showError(response.data?.message || t('保存失败'));
        return false;
      }
      showSuccess(payload.id ? t('监控分组已更新') : t('监控分组已创建'));
      setShowEdit(false);
      setEditingGroup(null);
      await loadGroups({ page: payload.id ? activePage : 1 });
      return true;
    } catch (error) {
      showError(error.response?.data?.message || t('保存失败'));
      return false;
    } finally {
      setSaving(false);
    }
  };

  const updateEnabled = async (group, enabled) => {
    const payload = normalizeMonitorGroupPayload(
      { ...getMonitorGroupFormValues(group), enabled },
      group,
    );
    setSaving(true);
    try {
      const response = await API.put('/api/monitor_group/', payload);
      if (!response.data?.success) {
        showError(response.data?.message || t('状态更新失败'));
        return;
      }
      showSuccess(enabled ? t('监控分组已启用') : t('监控分组已停用'));
      await loadGroups({ silent: true });
    } catch (error) {
      showError(error.response?.data?.message || t('状态更新失败'));
    } finally {
      setSaving(false);
    }
  };

  const deleteGroup = async (group) => {
    try {
      const response = await API.delete(`/api/monitor_group/${group.id}`);
      if (!response.data?.success) {
        showError(response.data?.message || t('删除失败'));
        return;
      }
      stopPolling(group.id);
      showSuccess(t('监控分组已删除'));
      await loadGroups({
        page: groups.length === 1 ? Math.max(1, activePage - 1) : activePage,
      });
    } catch (error) {
      showError(error.response?.data?.message || t('删除失败'));
    }
  };

  const openCreate = () => {
    setEditingGroup({});
    setShowEdit(true);
  };

  const openEdit = (group) => {
    setEditingGroup(group);
    setShowEdit(true);
  };

  const closeEdit = () => {
    setShowEdit(false);
    setEditingGroup(null);
  };

  const searchGroups = async () => {
    setSearching(true);
    await loadGroups({ page: 1, search: searchKeyword, silent: true });
    setSearching(false);
  };

  const resetSearch = async () => {
    setSearchKeyword('');
    setSearching(true);
    await loadGroups({ page: 1, search: '', silent: true });
    setSearching(false);
  };

  const handlePageChange = (page) => loadGroups({ page });
  const handlePageSizeChange = (size) => loadGroups({ page: 1, size });

  useEffect(() => {
    loadGroups({ page: 1 });
    loadChannels();
    return () => {
      pollingRef.current.forEach(({ timer }) => window.clearTimeout(timer));
      pollingRef.current.clear();
    };
    // Initial data is intentionally loaded once; paging and search are explicit.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return {
    t,
    groups,
    channels,
    loading,
    searching,
    saving,
    activePage,
    pageSize,
    total,
    searchKeyword,
    setSearchKeyword,
    compactMode,
    setCompactMode,
    showEdit,
    editingGroup,
    openCreate,
    openEdit,
    closeEdit,
    saveGroup,
    runGroup,
    updateEnabled,
    deleteGroup,
    searchGroups,
    resetSearch,
    loadGroups,
    handlePageChange,
    handlePageSizeChange,
  };
};
