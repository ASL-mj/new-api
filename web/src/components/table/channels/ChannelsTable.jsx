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

import React, { useMemo } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getChannelsColumns } from './ChannelsColumnDefs';

const ChannelsTable = (channelsData) => {
  const {
    channels,
    loading,
    searching,
    activePage,
    pageSize,
    channelCount,
    enableBatchDelete,
    compactMode,
    visibleColumns,
    setSelectedChannels,
    handlePageChange,
    handlePageSizeChange,
    handleRow,
    t,
    COLUMN_KEYS,
    // Column functions and data
    updateChannelBalance,
    manageChannel,
    manageTag,
    submitTagEdit,
    testChannel,
    setCurrentTestChannel,
    setShowModelTestModal,
    setEditingChannel,
    setShowEdit,
    setShowEditTag,
    setEditingTag,
    copySelectedChannel,
    refresh,
    checkOllamaVersion,
    // Multi-key management
    setShowMultiKeyManageModal,
    setCurrentMultiKeyChannel,
    openUpstreamUpdateModal,
    detectChannelUpstreamUpdates,
    usageStats,
    usageStatsLoading,
    usageStatsError,
    enableTagMode,
    clientSort,
    handleSortToggle,
    idSort,
  } = channelsData;

  // 客户端列排序（响应时间/今日消耗/限额/上游余额）：仅当前页、标签聚合模式下禁用
  const sortedChannels = useMemo(() => {
    if (!clientSort || enableTagMode) return channels;
    const { key, dir } = clientSort;
    const valueOf = (channel) => {
      if (key === 'response_time') return Number(channel.response_time || 0);
      const stat = usageStats[channel.id] || {};
      if (key === 'today_quota') return Number(stat.today_quota || 0);
      if (key === 'upstream_balance') return Number(stat.balance || 0);
      if (key === 'quota_limit') {
        if (stat.quota_limit_mode === 'key') {
          return stat.key_quota_unlimited
            ? Number.POSITIVE_INFINITY
            : Number(stat.key_quota_limit || 0);
        }
        return Number(stat.quota_limit || 0);
      }
      return 0;
    };
    return [...channels].sort((a, b) =>
      dir === 'asc' ? valueOf(a) - valueOf(b) : valueOf(b) - valueOf(a),
    );
  }, [channels, clientSort, usageStats, enableTagMode]);

  // Get all columns
  const allColumns = useMemo(() => {
    return getChannelsColumns({
      t,
      COLUMN_KEYS,
      sortState: { idSort, clientSort },
      onSortToggle: handleSortToggle,
      updateChannelBalance,
      manageChannel,
      manageTag,
      submitTagEdit,
      testChannel,
      setCurrentTestChannel,
      setShowModelTestModal,
      setEditingChannel,
      setShowEdit,
      setShowEditTag,
      setEditingTag,
      copySelectedChannel,
      refresh,
      activePage,
      channels,
      checkOllamaVersion,
      setShowMultiKeyManageModal,
      setCurrentMultiKeyChannel,
      openUpstreamUpdateModal,
      detectChannelUpstreamUpdates,
      usageStats,
      usageStatsLoading,
      usageStatsError,
    });
  }, [
    t,
    COLUMN_KEYS,
    idSort,
    clientSort,
    handleSortToggle,
    updateChannelBalance,
    manageChannel,
    manageTag,
    submitTagEdit,
    testChannel,
    setCurrentTestChannel,
    setShowModelTestModal,
    setEditingChannel,
    setShowEdit,
    setShowEditTag,
    setEditingTag,
    copySelectedChannel,
    refresh,
    activePage,
    channels,
    checkOllamaVersion,
    setShowMultiKeyManageModal,
    setCurrentMultiKeyChannel,
    openUpstreamUpdateModal,
    detectChannelUpstreamUpdates,
    usageStats,
    usageStatsLoading,
    usageStatsError,
  ]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    return allColumns.filter((column) => visibleColumns[column.key]);
  };

  const visibleColumnsList = useMemo(() => {
    return getVisibleColumns();
  }, [visibleColumns, allColumns]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? visibleColumnsList.map(({ fixed, ...rest }) => rest)
      : visibleColumnsList;
  }, [compactMode, visibleColumnsList]);

  return (
    <CardTable
      columns={tableColumns}
      dataSource={sortedChannels}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: channelCount,
        pageSizeOpts: [10, 20, 50, 100],
        showSizeChanger: true,
        onPageSizeChange: handlePageSizeChange,
        onPageChange: handlePageChange,
      }}
      hidePagination={true}
      expandAllRows={false}
      onRow={handleRow}
      rowSelection={
        enableBatchDelete
          ? {
              onChange: (selectedRowKeys, selectedRows) => {
                setSelectedChannels(selectedRows);
              },
            }
          : null
      }
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('搜索无结果')}
          style={{ padding: 30 }}
        />
      }
      className='rounded-xl overflow-hidden'
      size='middle'
      loading={loading || searching}
    />
  );
};

export default ChannelsTable;
