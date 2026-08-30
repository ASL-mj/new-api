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

import React, { useMemo } from 'react';
import { Empty, Spin } from '@douyinfe/semi-ui';
import { ExternalLink } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useLocation, useParams } from 'react-router-dom';
import { StatusContext } from '../../context/Status';
import {
  isSafeSidebarUrl,
  normalizeSidebarCustomItems,
} from '../../hooks/common/useSidebar';

const External = () => {
  const { t } = useTranslation();
  const { id } = useParams();
  const location = useLocation();
  const [statusState] = React.useContext(StatusContext);
  const isSidebarEmbedded = location.pathname.startsWith('/console/external/');
  const contentClassName = isSidebarEmbedded
    ? 'mt-[60px] h-[calc(100vh-112px)] w-full overflow-hidden rounded-lg border border-semi-color-border bg-semi-color-bg-0'
    : 'mt-16 h-[calc(100vh-64px)] w-full overflow-hidden';

  const item = useMemo(() => {
    if (!statusState?.status?.SidebarModulesAdmin) return null;
    try {
      const config = JSON.parse(statusState.status.SidebarModulesAdmin);
      return normalizeSidebarCustomItems(config.custom).find(
        (customItem) => customItem.id === id,
      );
    } catch {
      return null;
    }
  }, [id, statusState?.status?.SidebarModulesAdmin]);

  if (statusState?.status === undefined) {
    return (
      <div className={`${contentClassName} flex items-center justify-center`}>
        <Spin spinning />
      </div>
    );
  }

  if (!item || !item.enabled || !isSafeSidebarUrl(item.url)) {
    return (
      <div
        className={`${contentClassName} flex items-center justify-center p-8`}
      >
        <Empty description={t('外部平台地址未配置或不可用')} />
      </div>
    );
  }

  return (
    <div className={`${contentClassName} relative`}>
      {/* 部分站点会通过 X-Frame-Options / CSP 拒绝被 iframe 嵌入，提供新窗口打开兜底 */}
      <button
        type='button'
        className='absolute right-4 top-3 z-10 inline-flex items-center gap-1.5 rounded-full border border-semi-color-border bg-semi-color-bg-0 px-3 py-1.5 text-xs font-medium text-semi-color-text-0 shadow-sm transition-colors hover:bg-semi-color-fill-0'
        onClick={() => window.open(item.url, '_blank', 'noopener,noreferrer')}
      >
        <ExternalLink size={14} />
        {t('新窗口打开')}
      </button>
      <iframe
        src={item.url}
        title={item.name}
        className='h-full w-full border-none'
        referrerPolicy='strict-origin-when-cross-origin'
      />
    </div>
  );
};

export default External;
