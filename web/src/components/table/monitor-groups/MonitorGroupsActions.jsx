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
import { Button } from '@douyinfe/semi-ui';
import { Plus } from 'lucide-react';

const MonitorGroupsActions = ({ openCreate, t }) => (
  <div className='flex w-full flex-wrap gap-2 md:w-auto'>
    <Button
      type='primary'
      size='small'
      icon={<Plus size={16} />}
      onClick={openCreate}
      className='w-full md:w-auto'
    >
      {t('新增渠道监控')}
    </Button>
  </div>
);

export default MonitorGroupsActions;
