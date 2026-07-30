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

import React, { useEffect, useState } from 'react';
import { SideSheet, Typography, TabPane, Tabs } from '@douyinfe/semi-ui';
import { IconClose } from '@douyinfe/semi-icons';

import { useIsMobile } from '../../../../hooks/common/useIsMobile';
import ModelHeader from './components/ModelHeader';
import ModelOverviewTab from './components/ModelOverviewTab';
import ModelApiTab from './components/ModelApiTab';
import ModelPerformancePanel from './components/ModelPerformancePanel';

const { Text } = Typography;

const ModelDetailSideSheet = ({
  visible,
  onClose,
  modelData,
  groupRatio,
  currency,
  siteDisplayType,
  tokenUnit,
  displayPrice,
  showRatio,
  usableGroup,
  vendorsMap,
  endpointMap,
  autoGroups,
  t,
}) => {
  const isMobile = useIsMobile();
  const [activeTab, setActiveTab] = useState('overview');
  const modelName = modelData?.model_name || '';

  useEffect(() => {
    setActiveTab('overview');
  }, [modelName, visible]);

  return (
    <SideSheet
      placement='right'
      title={
        <ModelHeader modelData={modelData} vendorsMap={vendorsMap} t={t} />
      }
      bodyStyle={{
        padding: '0',
        display: 'flex',
        flexDirection: 'column',
        borderBottom: '1px solid var(--semi-color-border)',
      }}
      visible={visible}
      width={isMobile ? '100%' : 760}
      closeIcon={<IconClose />}
      onCancel={onClose}
    >
      <div style={{ padding: '0 24px 24px' }}>
        {!modelData && (
          <div className='flex justify-center items-center py-10'>
            <Text type='secondary'>{t('加载中...')}</Text>
          </div>
        )}
        {modelData && (
          <Tabs type='line' activeKey={activeTab} onChange={setActiveTab}>
            <TabPane tab={t('概览')} itemKey='overview'>
              {activeTab === 'overview' && (
                <div style={{ paddingTop: 16 }}>
                  <ModelOverviewTab
                    modelData={modelData}
                    groupRatio={groupRatio}
                    currency={currency}
                    siteDisplayType={siteDisplayType}
                    tokenUnit={tokenUnit}
                    displayPrice={displayPrice}
                    showRatio={showRatio}
                    usableGroup={usableGroup}
                    autoGroups={autoGroups}
                    vendorsMap={vendorsMap}
                    t={t}
                  />
                </div>
              )}
            </TabPane>
            <TabPane tab={t('性能')} itemKey='performance'>
              {activeTab === 'performance' && (
                <div style={{ paddingTop: 16 }}>
                  <ModelPerformancePanel modelName={modelName} t={t} />
                </div>
              )}
            </TabPane>
            <TabPane tab='API' itemKey='api'>
              {activeTab === 'api' && (
                <div style={{ paddingTop: 16 }}>
                  <ModelApiTab
                    modelData={modelData}
                    endpointMap={endpointMap}
                    t={t}
                  />
                </div>
              )}
            </TabPane>
          </Tabs>
        )}
      </div>
    </SideSheet>
  );
};

export default ModelDetailSideSheet;
