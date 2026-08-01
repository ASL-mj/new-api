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
import React from 'react';

import CardPro from '../../common/ui/CardPro';
import { useMonitorGroupsData } from '../../../hooks/monitor-groups/useMonitorGroupsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import MonitorGroupsActions from './MonitorGroupsActions';
import MonitorGroupsDescription from './MonitorGroupsDescription';
import MonitorGroupsFilters from './MonitorGroupsFilters';
import MonitorGroupsTable from './MonitorGroupsTable';
import EditMonitorGroupModal from './modals/EditMonitorGroupModal';

const MonitorGroupsPage = () => {
  const data = useMonitorGroupsData();
  const isMobile = useIsMobile();

  return (
    <>
      <EditMonitorGroupModal {...data} visible={data.showEdit} />
      <CardPro
        type='type1'
        descriptionArea={
          <MonitorGroupsDescription
            compactMode={data.compactMode}
            setCompactMode={data.setCompactMode}
            t={data.t}
          />
        }
        actionsArea={
          <div className='flex w-full flex-col items-center justify-between gap-2 md:flex-row'>
            <MonitorGroupsActions openCreate={data.openCreate} t={data.t} />
            <MonitorGroupsFilters {...data} />
          </div>
        }
        paginationArea={createCardProPagination({
          currentPage: data.activePage,
          pageSize: data.pageSize,
          total: data.total,
          onPageChange: data.handlePageChange,
          onPageSizeChange: data.handlePageSizeChange,
          isMobile,
          t: data.t,
        })}
        t={data.t}
      >
        <MonitorGroupsTable {...data} />
      </CardPro>
    </>
  );
};

export default MonitorGroupsPage;
