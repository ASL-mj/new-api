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
  Descriptions,
  Divider,
  Modal,
  Typography,
} from '@douyinfe/semi-ui';
import { RefreshCw, Video } from 'lucide-react';
import { isRoot } from '../../../../helpers';
import { safeTaskUrl } from '../../../../helpers/h3Task';
import TaskBillingDetails from '../TaskBillingDetails';
import { renderStatus } from '../TaskLogsColumnDefs';

const TaskDetailModal = ({
  task,
  onClose,
  isAdminUser,
  openVideoModal,
  refresh,
  loading,
  t,
}) => {
  if (!task) return null;
  const url = safeTaskUrl(task.result_url);
  return (
    <Modal
      visible
      title={t('任务详情')}
      footer={null}
      onCancel={onClose}
      style={{ width: 640, maxWidth: 'calc(100vw - 24px)' }}
    >
      <Descriptions
        column={1}
        style={{ overflowWrap: 'anywhere' }}
        data={[
          {
            key: t('任务ID'),
            value: <Typography.Text copyable>{task.task_id}</Typography.Text>,
          },
          {
            key: t('模型'),
            value: task.model || task.properties?.origin_model_name || '-',
          },
          { key: t('任务状态'), value: renderStatus(task.status, t) },
          { key: t('进度'), value: task.progress || '-' },
          ...(task.fail_reason && task.status !== 'SUCCESS'
            ? [{ key: t('失败原因'), value: task.fail_reason }]
            : []),
        ]}
      />
      <Divider />
      <div className='mb-4'>
        <Button
          icon={<RefreshCw size={16} />}
          loading={loading}
          onClick={refresh}
        >
          {t('刷新')}
        </Button>
      </div>
      <TaskBillingDetails
        task={task}
        admin={isAdminUser}
        root={isRoot()}
        t={t}
      />
      {task.status === 'SUCCESS' && url && (
        <div className='mt-4 flex flex-wrap gap-2 items-start'>
          <Button
            icon={<Video size={16} />}
            onClick={() => openVideoModal(url)}
          >
            {t('点击预览视频')}
          </Button>
          <Typography.Text
            copyable
            style={{ overflowWrap: 'anywhere', maxWidth: '100%' }}
          >
            {url}
          </Typography.Text>
        </div>
      )}
    </Modal>
  );
};
export default TaskDetailModal;
