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

import React, { useCallback, useContext, useEffect, useRef } from 'react';
import { Empty, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';

const Docs = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const iframeRef = useRef(null);
  const docsLink = statusState?.status?.docs_link || '';

  const sendFrameContext = useCallback(() => {
    iframeRef.current?.contentWindow?.postMessage(
      { themeMode: actualTheme },
      '*',
    );
    iframeRef.current?.contentWindow?.postMessage({ lang: i18n.language }, '*');
  }, [actualTheme, i18n.language]);

  useEffect(() => {
    sendFrameContext();
  }, [sendFrameContext]);

  if (statusState?.status === undefined) {
    return (
      <div className='mt-16 flex h-[calc(100vh-64px)] items-center justify-center'>
        <Spin spinning />
      </div>
    );
  }

  if (!docsLink) {
    return (
      <div className='mt-16 flex h-[calc(100vh-64px)] items-center justify-center p-8'>
        <Empty description={t('文档地址未配置')} />
      </div>
    );
  }

  return (
    <div className='mt-16 h-[calc(100vh-64px)] w-full overflow-hidden'>
      <iframe
        ref={iframeRef}
        src={docsLink}
        title={t('文档')}
        className='h-full w-full border-none'
        onLoad={sendFrameContext}
      />
    </div>
  );
};

export default Docs;
