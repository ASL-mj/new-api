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
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';

import CardTable from '../../common/ui/CardTable';
import { getMonitorGroupsColumns } from './MonitorGroupsColumnDefs';

const MonitorGroupsTable = (monitorGroupsData) => {
  const {
    groups,
    loading,
    searching,
    compactMode,
    activePage,
    pageSize,
    total,
    handlePageChange,
    handlePageSizeChange,
    t,
  } = monitorGroupsData;

  const columns = useMemo(
    () => getMonitorGroupsColumns(monitorGroupsData),
    [
      monitorGroupsData.runGroup,
      monitorGroupsData.updateEnabled,
      monitorGroupsData.openEdit,
      monitorGroupsData.deleteGroup,
      monitorGroupsData.saving,
      t,
    ],
  );
  const tableColumns = useMemo(
    () =>
      compactMode ? columns.map(({ fixed, ...column }) => column) : columns,
    [columns, compactMode],
  );

  return (
    <CardTable
      columns={tableColumns}
      dataSource={groups}
      rowKey='id'
      scroll={compactMode ? undefined : { x: 'max-content' }}
      pagination={{
        currentPage: activePage,
        pageSize,
        total,
        pageSizeOpts: [10, 20, 50, 100],
        showSizeChanger: true,
        onPageChange: handlePageChange,
        onPageSizeChange: handlePageSizeChange,
      }}
      hidePagination
      loading={loading || searching}
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('暂无渠道监控分组')}
          style={{ padding: 30 }}
        />
      }
      className='overflow-hidden rounded-xl'
      size='middle'
    />
  );
};

export default MonitorGroupsTable;
