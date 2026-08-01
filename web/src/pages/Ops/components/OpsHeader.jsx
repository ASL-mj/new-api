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
  Card,
  Input,
  Radio,
  RadioGroup,
  Select,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { RefreshCw, RotateCcw, Search } from 'lucide-react';

import { CHANNEL_OPTIONS } from '../../../constants';
import { selectFilter } from '../../../helpers';
import { OPS_PERIOD_OPTIONS } from '../../../hooks/ops/useOpsData';

const OpsHeader = ({
  draftFilters,
  setDraftFilters,
  applyFilters,
  resetFilters,
  refresh,
  refreshing,
  t,
}) => (
  <>
    <div className='mb-4 flex items-center justify-between gap-3'>
      <Typography.Title heading={4} style={{ margin: 0 }}>
        {t('运维监控')}
      </Typography.Title>
      <Tooltip content={t('刷新全部数据')}>
        <Button
          icon={<RefreshCw size={16} />}
          theme='borderless'
          type='tertiary'
          loading={refreshing}
          aria-label={t('刷新全部数据')}
          onClick={refresh}
        />
      </Tooltip>
    </div>
    <Card className='mb-3 !rounded-lg' bodyStyle={{ padding: 16 }}>
      <div className='flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between'>
        <RadioGroup
          type='button'
          size='small'
          value={draftFilters.period}
          onChange={(event) =>
            setDraftFilters((current) => ({
              ...current,
              period: event.target.value,
            }))
          }
        >
          {OPS_PERIOD_OPTIONS.map((option) => (
            <Radio key={option.value} value={option.value}>
              {t(option.label)}
            </Radio>
          ))}
        </RadioGroup>
        <div className='flex flex-col gap-2 sm:flex-row sm:flex-wrap'>
          <Select
            size='small'
            value={draftFilters.channelType}
            optionList={CHANNEL_OPTIONS}
            filter={selectFilter}
            showClear
            placeholder={t('渠道类型')}
            style={{ width: 160 }}
            onChange={(value) =>
              setDraftFilters((current) => ({
                ...current,
                channelType: value,
              }))
            }
          />
          <Input
            size='small'
            value={draftFilters.group}
            placeholder={t('调用分组')}
            style={{ width: 140 }}
            showClear
            onChange={(value) =>
              setDraftFilters((current) => ({ ...current, group: value }))
            }
          />
          <Input
            size='small'
            value={draftFilters.model}
            placeholder={t('模型名称')}
            style={{ width: 170 }}
            showClear
            onChange={(value) =>
              setDraftFilters((current) => ({ ...current, model: value }))
            }
          />
          <Button
            type='primary'
            size='small'
            icon={<Search size={14} />}
            onClick={applyFilters}
          >
            {t('应用筛选')}
          </Button>
          <Button
            type='tertiary'
            size='small'
            icon={<RotateCcw size={14} />}
            onClick={resetFilters}
          >
            {t('重置')}
          </Button>
        </div>
      </div>
    </Card>
  </>
);

export default OpsHeader;
