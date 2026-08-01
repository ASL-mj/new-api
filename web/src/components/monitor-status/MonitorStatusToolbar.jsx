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
import {
  Button,
  Radio,
  RadioGroup,
  Space,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { LayoutGrid, List, RefreshCw } from 'lucide-react';

const MonitorStatusToolbar = ({
  days,
  setDays,
  view,
  setView,
  refresh,
  refreshing,
  refreshAfter,
  t,
}) => (
  <div className='mb-4 flex flex-col items-start justify-between gap-3 lg:flex-row lg:items-center'>
    <Typography.Title heading={4} style={{ margin: 0 }}>
      {t('分组状态')}
    </Typography.Title>
    <div className='flex w-full flex-wrap items-center gap-2 lg:w-auto lg:justify-end'>
      <Tooltip content={t('刷新')}>
        <Button
          icon={<RefreshCw size={15} />}
          theme='borderless'
          type='tertiary'
          size='small'
          loading={refreshing}
          aria-label={t('刷新')}
          onClick={refresh}
        />
      </Tooltip>
      <Tag color='blue' shape='circle'>
        {t('自动刷新：{{seconds}} s', { seconds: refreshAfter })}
      </Tag>
      <RadioGroup
        type='button'
        value={days}
        onChange={(event) => setDays(event.target.value)}
        size='small'
      >
        <Radio value='7'>{t('7 天')}</Radio>
        <Radio value='15'>{t('15 天')}</Radio>
        <Radio value='30'>{t('30 天')}</Radio>
      </RadioGroup>
      <RadioGroup
        type='button'
        value={view}
        onChange={(event) => setView(event.target.value)}
        size='small'
      >
        <Radio value='card'>
          <Space spacing={4}>
            <LayoutGrid size={14} />
            {t('卡片')}
          </Space>
        </Radio>
        <Radio value='list'>
          <Space spacing={4}>
            <List size={14} />
            {t('列表')}
          </Space>
        </Radio>
      </RadioGroup>
    </div>
  </div>
);

export default MonitorStatusToolbar;
