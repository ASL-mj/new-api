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
  Banner,
  Avatar,
  Button,
  Empty,
  Skeleton,
  Typography,
} from '@douyinfe/semi-ui';
import { IconPulse } from '@douyinfe/semi-icons';
import { useModelPerformance } from '../../../../../hooks/model-pricing/useModelPerformance';
import { formatRequestCount } from '../../../../../helpers/performance';
import ModelPerformanceStats from './ModelPerformanceStats';
import ModelPerformanceGroupTable from './ModelPerformanceGroupTable';
import ModelPerformanceCharts from './ModelPerformanceCharts';

const { Text } = Typography;

const ModelPerformancePanel = ({ modelName, t }) => {
  const { status, data, error, retry } = useModelPerformance({
    modelName,
    hours: 24,
    enabled: Boolean(modelName),
  });

  const overall = data?.overall;
  const hasData = Number(overall?.request_count) > 0;
  const hasSparseSample = hasData && overall.request_count < 10;

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div className='flex items-center'>
          <Avatar size='small' color='green' className='mr-2 shadow-md'>
            <IconPulse size={16} />
          </Avatar>
          <div>
            <Text className='text-lg font-medium'>{t('模型性能')}</Text>
            <div className='text-xs text-gray-600 mt-1'>
              {t('最近 24 小时的性能指标')} · {t('仅统计真实 Relay 请求')}
            </div>
          </div>
        </div>
      </div>

      {status === 'loading' && !data && (
        <div className='space-y-4'>
          <div
            className='grid gap-2'
            style={{
              gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))',
            }}
          >
            {Array.from({ length: 3 }, (_, index) => (
              <Skeleton
                key={index}
                active
                placeholder={<Skeleton.Paragraph rows={2} />}
              />
            ))}
          </div>
          <Skeleton active placeholder={<Skeleton.Paragraph rows={8} />} />
        </div>
      )}

      {status === 'error' && !data && (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          title={t('性能数据加载失败')}
          description={error?.message || t('请稍后重试')}
          style={{ padding: '72px 16px' }}
        >
          <Button type='primary' onClick={retry}>
            {t('重新加载性能数据')}
          </Button>
        </Empty>
      )}

      {data && !hasData && (
        <div className='space-y-3'>
          {status === 'loading' && (
            <Banner type='info' description={t('正在刷新性能数据')} />
          )}
          {status === 'error' && (
            <Banner
              type='danger'
              description={error?.message || t('上次数据仍在显示，刷新失败。')}
              closeIcon={null}
            />
          )}
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            title={t('暂无性能数据')}
            description={t(
              '此时间范围内暂无模型性能数据。仅统计真实 Relay 请求，请在模型产生调用后再查看。',
            )}
            style={{ padding: '72px 16px' }}
          >
            <Button
              type='primary'
              loading={status === 'loading'}
              disabled={status === 'loading'}
              onClick={retry}
            >
              {t('重新加载性能数据')}
            </Button>
          </Empty>
        </div>
      )}

      {data && hasData && (
        <>
          {status === 'loading' && (
            <Banner type='info' description={t('正在刷新性能数据')} />
          )}
          {status === 'error' && (
            <Banner
              type='danger'
              description={error?.message || t('上次数据仍在显示，刷新失败。')}
              closeIcon={null}
            />
          )}
          {hasSparseSample && (
            <Banner
              type='warning'
              description={t(
                '样本较少：当前仅有 {{count}} 个有效请求，指标可能会波动。',
                {
                  count: formatRequestCount(overall.request_count),
                },
              )}
              closeIcon={null}
            />
          )}
          <div className='flex items-center justify-between'>
            <Text type='tertiary' size='small'>
              {t('有效请求')}：{formatRequestCount(overall.request_count)}
            </Text>
            <Button
              theme='borderless'
              size='small'
              loading={status === 'loading'}
              disabled={status === 'loading'}
              onClick={retry}
            >
              {t('重新加载性能数据')}
            </Button>
          </div>
          <ModelPerformanceStats overall={overall} t={t} />
          <ModelPerformanceGroupTable groups={data.groups || []} t={t} />
          <ModelPerformanceCharts
            series={overall.series || []}
            groupSeries={data.groups || []}
            t={t}
          />
        </>
      )}
    </div>
  );
};

export default ModelPerformancePanel;
