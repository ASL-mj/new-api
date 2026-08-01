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
import React, { useEffect, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Form,
  Modal,
  Row,
  Space,
  Tag,
} from '@douyinfe/semi-ui';

import { getMonitorGroupFormValues } from '../../../../hooks/monitor-groups/useMonitorGroupsData';
import { getCommonChannelModels } from '../../../../hooks/monitor-groups/monitorGroupUtils';

const EditMonitorGroupModal = ({
  visible,
  editingGroup,
  channels,
  saving,
  closeEdit,
  saveGroup,
  t,
}) => {
  const formApiRef = useRef(null);
  const [selectedChannelIds, setSelectedChannelIds] = useState([]);
  const isEdit = Number(editingGroup?.id) > 0;
  const selectedChannels = channels.filter((channel) =>
    selectedChannelIds.includes(channel.id),
  );
  const commonModels = getCommonChannelModels(selectedChannels);
  const modelOptions = commonModels.map((model) => ({
    label: model,
    value: model,
  }));
  const noCommonModels =
    selectedChannelIds.length > 0 && commonModels.length === 0;

  useEffect(() => {
    if (!visible) return;
    const values = getMonitorGroupFormValues(editingGroup);
    setSelectedChannelIds(values.channel_ids);
  }, [visible, editingGroup]);

  const handleChannelsChange = (values) => {
    const channelIds = (values || []).map(Number);
    setSelectedChannelIds(channelIds);
    const allowed = new Set(
      getCommonChannelModels(
        channels.filter((channel) => channelIds.includes(channel.id)),
      ),
    );
    const current = formApiRef.current?.getValues() || {};
    if (!allowed.has(current.primary_model)) {
      formApiRef.current?.setValue('primary_model', undefined);
    }
    formApiRef.current?.setValue(
      'extra_models',
      (current.extra_models || []).filter((model) => allowed.has(model)),
    );
  };

  const channelOptions = channels.map((channel) => ({
    value: channel.id,
    label: (
      <div className='flex min-w-0 items-center justify-between gap-3'>
        <span className='truncate'>
          #{channel.id} {channel.name || t('未命名渠道')}
        </span>
        <Space spacing={4}>
          <Tag size='small' color='blue'>
            {channel.type_name || t('未知')}
          </Tag>
          {channel.status !== 1 && (
            <Tag size='small' color='grey'>
              {t('已停用')}
            </Tag>
          )}
        </Space>
      </div>
    ),
  }));

  return (
    <Modal
      visible={visible}
      title={isEdit ? t('编辑渠道监控') : t('新增渠道监控')}
      width={760}
      onCancel={closeEdit}
      footer={
        <Space>
          <Button onClick={closeEdit}>{t('取消')}</Button>
          <Button
            type='primary'
            loading={saving}
            disabled={noCommonModels || selectedChannelIds.length === 0}
            onClick={() => formApiRef.current?.submitForm()}
          >
            {t('保存')}
          </Button>
        </Space>
      }
    >
      {visible && (
        <Form
          key={
            isEdit ? `monitor-group-${editingGroup.id}` : 'monitor-group-new'
          }
          initValues={getMonitorGroupFormValues(editingGroup)}
          getFormApi={(api) => {
            formApiRef.current = api;
          }}
          onSubmit={saveGroup}
        >
          <Row gutter={12}>
            <Col span={12} xs={24}>
              <Form.Input
                field='name'
                label={t('分组名称')}
                placeholder={t('例如：Codex 主渠道')}
                rules={[{ required: true, message: t('请输入分组名称') }]}
              />
            </Col>
            <Col span={12} xs={24}>
              <Form.Input
                field='key'
                label={t('分组标识')}
                placeholder='codex-main'
                disabled={isEdit}
                rules={[{ required: true, message: t('请输入分组标识') }]}
                extraText={
                  isEdit
                    ? t('创建后不可修改')
                    : t('仅支持小写字母、数字、下划线和连字符')
                }
              />
            </Col>
            <Col span={24}>
              <Form.Input
                field='description'
                label={t('说明')}
                maxLength={255}
                showClear
              />
            </Col>
            <Col span={24}>
              <Form.Select
                field='channel_ids'
                label={t('探测渠道')}
                multiple
                filter
                maxTagCount={3}
                optionList={channelOptions}
                placeholder={t('先选择要监控的渠道')}
                onChange={handleChannelsChange}
                rules={[{ required: true, message: t('至少选择一个渠道') }]}
              />
            </Col>
            {noCommonModels && (
              <Col span={24}>
                <Banner
                  type='warning'
                  description={t(
                    '所选渠道没有共同支持的模型，请调整渠道选择。',
                  )}
                  className='mb-4'
                />
              </Col>
            )}
            <Col span={12} xs={24}>
              <Form.Select
                field='primary_model'
                label={t('主探测模型')}
                optionList={modelOptions}
                filter
                disabled={commonModels.length === 0}
                placeholder={t('选择主探测模型')}
                rules={[{ required: true, message: t('请选择主探测模型') }]}
              />
            </Col>
            <Col span={12} xs={24}>
              <Form.Select
                field='extra_models'
                label={t('额外探测模型')}
                optionList={modelOptions}
                multiple
                filter
                maxTagCount={3}
                disabled={commonModels.length === 0}
                placeholder={t('可选')}
              />
            </Col>
            <Col span={8} xs={24}>
              <Form.InputNumber
                field='interval_seconds'
                label={t('探测间隔（秒）')}
                min={15}
                max={3600}
                precision={0}
                style={{ width: '100%' }}
              />
            </Col>
            <Col span={8} xs={24}>
              <Form.InputNumber
                field='timeout_seconds'
                label={t('超时（秒）')}
                min={5}
                max={120}
                precision={0}
                style={{ width: '100%' }}
              />
            </Col>
            <Col span={8} xs={24}>
              <Form.InputNumber
                field='degraded_ms'
                label={t('降级阈值（毫秒）')}
                min={1}
                max={300000}
                precision={0}
                style={{ width: '100%' }}
              />
            </Col>
            <Col span={12} xs={24}>
              <Form.Switch field='enabled' label={t('启用定时探测')} />
            </Col>
            <Col span={12} xs={24}>
              <Form.Switch field='user_visible' label={t('向用户展示')} />
            </Col>
          </Row>
        </Form>
      )}
    </Modal>
  );
};

export default EditMonitorGroupModal;
