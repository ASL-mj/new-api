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
import { useLocation, useNavigate } from 'react-router-dom';
import { Layout } from '@douyinfe/semi-ui';
import CardPro from '../../common/ui/CardPro';
import TaskLogsTable from './TaskLogsTable';
import TaskLogsActions from './TaskLogsActions';
import TaskLogsFilters from './TaskLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import ContentModal from './modals/ContentModal';
import AudioPreviewModal from './modals/AudioPreviewModal';
import H3GenerationModal from './H3GenerationModal';
import TaskDetailModal from './modals/TaskDetailModal';
import { useTaskLogsData } from '../../../hooks/task-logs/useTaskLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import { taskDetailHref } from '../../../helpers/h3Task';

const TaskLogsPage = () => {
  const location = useLocation();
  const navigate = useNavigate();
  const taskId = React.useMemo(
    () => new URLSearchParams(location.search).get('task_id')?.trim() || '',
    [location.search],
  );
  const taskLogsData = useTaskLogsData({ initialTaskId: taskId });
  const isMobile = useIsMobile();
  const [showH3, setShowH3] = React.useState(false);
  const [detailTask, setDetailTask] = React.useState(null);

  React.useEffect(() => {
    if (!taskId) {
      setDetailTask(null);
      return;
    }
    setDetailTask(
      taskLogsData.logs.find((task) => task.task_id === taskId) || null,
    );
  }, [taskId, taskLogsData.logs]);

  const openTaskDetail = React.useCallback(
    (task) => {
      const nextTaskId = String(task?.task_id || '').trim();
      if (!nextTaskId) return;
      setDetailTask(task);
      navigate(taskDetailHref(nextTaskId));
    },
    [navigate],
  );

  const closeTaskDetail = React.useCallback(() => {
    setDetailTask(null);
    navigate(location.pathname, { replace: true });
  }, [location.pathname, navigate]);

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...taskLogsData} />
      <TaskDetailModal
        task={detailTask}
        onClose={closeTaskDetail}
        isAdminUser={taskLogsData.isAdminUser}
        refresh={taskLogsData.refresh}
        loading={taskLogsData.loading}
        openVideoModal={taskLogsData.openVideoModal}
        t={taskLogsData.t}
      />
      <ContentModal {...taskLogsData} isVideo={false} />
      {/* 新增：视频预览弹窗 */}
      <ContentModal
        isModalOpen={taskLogsData.isVideoModalOpen}
        setIsModalOpen={taskLogsData.setIsVideoModalOpen}
        modalContent={taskLogsData.videoUrl}
        isVideo={true}
      />
      <AudioPreviewModal
        isModalOpen={taskLogsData.isAudioModalOpen}
        setIsModalOpen={taskLogsData.setIsAudioModalOpen}
        audioClips={taskLogsData.audioClips}
      />
      {showH3 && (
        <H3GenerationModal
          onClose={() => setShowH3(false)}
          onSubmitted={taskLogsData.refresh}
          t={taskLogsData.t}
        />
      )}

      <Layout>
        <CardPro
          type='type2'
          statsArea={
            <TaskLogsActions
              {...taskLogsData}
              onGenerate={() => setShowH3(true)}
            />
          }
          searchArea={<TaskLogsFilters {...taskLogsData} />}
          paginationArea={createCardProPagination({
            currentPage: taskLogsData.activePage,
            pageSize: taskLogsData.pageSize,
            total: taskLogsData.logCount,
            onPageChange: taskLogsData.handlePageChange,
            onPageSizeChange: taskLogsData.handlePageSizeChange,
            isMobile: isMobile,
            t: taskLogsData.t,
          })}
          t={taskLogsData.t}
        >
          <TaskLogsTable {...taskLogsData} openTaskDetail={openTaskDetail} />
        </CardPro>
      </Layout>
    </>
  );
};

export default TaskLogsPage;
