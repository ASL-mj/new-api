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
import { Button, Input } from '@douyinfe/semi-ui';
import { IconRefresh, IconSearch } from '@douyinfe/semi-icons';

const MonitorGroupsFilters = ({
  searchKeyword,
  setSearchKeyword,
  searchGroups,
  resetSearch,
  loading,
  searching,
  t,
}) => {
  const submitOnEnter = (event) => {
    if (event.key === 'Enter') searchGroups();
  };

  return (
    <div className='flex w-full flex-col items-center gap-2 md:w-auto md:flex-row'>
      <Input
        value={searchKeyword}
        onChange={setSearchKeyword}
        onKeyDown={submitOnEnter}
        prefix={<IconSearch />}
        placeholder={t('搜索名称或标识')}
        showClear
        size='small'
        className='w-full md:w-56'
      />
      <div className='flex w-full gap-2 md:w-auto'>
        <Button
          size='small'
          type='primary'
          icon={<IconSearch />}
          loading={searching}
          disabled={loading}
          onClick={searchGroups}
          className='flex-1 md:flex-initial'
        >
          {t('查询')}
        </Button>
        <Button
          size='small'
          type='tertiary'
          icon={<IconRefresh />}
          disabled={loading || searching}
          onClick={resetSearch}
          className='flex-1 md:flex-initial'
        >
          {t('重置')}
        </Button>
      </div>
    </div>
  );
};

export default MonitorGroupsFilters;
