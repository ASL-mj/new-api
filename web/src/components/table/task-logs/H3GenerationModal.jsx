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
  Form,
  Input,
  Modal,
  Select,
  Tooltip,
} from '@douyinfe/semi-ui';
import { Plus, Trash2, Video } from 'lucide-react';
import { API, showSuccess } from '../../../helpers';
import {
  buildH3Submission,
  H3_RATIOS,
  h3TokenOptions,
} from '../../../helpers/h3Task';

const H3GenerationModal = ({ onClose, onSubmitted, t }) => {
  const [media, setMedia] = useState([]);
  const [tokens, setTokens] = useState([]);
  const [models, setModels] = useState([]);
  const [selectedTokenId, setSelectedTokenId] = useState(null);
  const [tokenPage, setTokenPage] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [loadingTokens, setLoadingTokens] = useState(true);
  const [loadingModels, setLoadingModels] = useState(false);
  const [tokenError, setTokenError] = useState('');
  const [modelError, setModelError] = useState('');
  const loadError = tokenError || modelError;
  const [submitError, setSubmitError] = useState('');
  const [saving, setSaving] = useState(false);
  const submitting = useRef(false);
  const modelRequestId = useRef(0);
  const formApiRef = useRef(null);
  const mounted = useRef(true);
  const nextMediaId = useRef(0);
  const mediaRoles = {
    first_frame: t('首帧图片'),
    last_frame: t('尾帧图片'),
    reference_image: t('参考图片'),
    reference_video: t('参考视频'),
    reference_audio: t('参考音频'),
  };
  const loadTokens = async (page = 1) => {
    setLoadingTokens(true);
    setTokenError('');
    try {
      const { data: response } = await API.get('/api/token/', {
        params: { p: page, size: 20 },
        skipErrorHandler: true,
      });
      if (!response.success) throw new Error(response.message || t('加载失败'));
      if (!mounted.current) return;
      const options = h3TokenOptions(response.data.items || []);
      setTokens((current) =>
        page === 1
          ? options
          : [
              ...current,
              ...options.filter(
                (option) =>
                  !current.some((item) => item.value === option.value),
              ),
            ],
      );
      setTokenPage(page);
      setHasMore(page * 20 < response.data.total);
    } catch {
      if (mounted.current) setTokenError(t('令牌加载失败'));
    } finally {
      if (mounted.current) setLoadingTokens(false);
    }
  };
  const loadModels = async (tokenId) => {
    const requestId = ++modelRequestId.current;
    setLoadingModels(true);
    setModelError('');
    setModels([]);
    try {
      const { data: response } = await API.get('/api/task/video/models', {
        params: { token_id: tokenId },
        skipErrorHandler: true,
      });
      if (!response.success) throw new Error();
      if (!mounted.current || requestId !== modelRequestId.current) return;
      const available = Array.isArray(response.data) ? response.data : [];
      setModels(available);
      const current = formApiRef.current?.getValue('model');
      formApiRef.current?.setValue(
        'model',
        available.includes(current) ? current : available[0],
      );
    } catch {
      if (mounted.current && requestId === modelRequestId.current)
        setModelError(t('模型加载失败'));
    } finally {
      if (mounted.current && requestId === modelRequestId.current)
        setLoadingModels(false);
    }
  };
  useEffect(() => {
    mounted.current = true;
    loadTokens();
    return () => {
      mounted.current = false;
    };
  }, []);
  const updateMedia = (id, key, value) =>
    setMedia((current) =>
      current.map((item) =>
        item.id === id ? { ...item, [key]: value } : item,
      ),
    );
  const submit = async (values) => {
    if (submitting.current) return;
    setSubmitError('');
    let payload;
    try {
      payload = buildH3Submission({ ...values, media });
      if (
        !models.includes(values.model) ||
        !tokens.some(
          (token) => token.value === values.token_id && !token.disabled,
        )
      )
        throw new Error('请选择有效令牌和模型');
    } catch (error) {
      setSubmitError(t(error.message));
      return;
    }
    submitting.current = true;
    setSaving(true);
    try {
      const res = await API.post('/api/task/video', payload, {
        skipErrorHandler: true,
      });
      if (!res.data.success) throw new Error(res.data.message || t('提交失败'));
      if (!res.data.data?.task_id)
        throw new Error(t('提交状态待确认，请先刷新任务记录'));
      showSuccess(t('任务已提交') + ': ' + res.data.data.task_id);
      onClose();
      // A list refresh failure must never be reported as a failed submission.
      await onSubmitted(res.data.data.task_id);
    } catch (error) {
      const message =
        error.response?.data?.message ||
        (error.isAxiosError
          ? t('提交状态待确认，请先刷新任务记录')
          : error.message);
      if (mounted.current)
        setSubmitError(message || t('提交状态待确认，请先刷新任务记录'));
    } finally {
      submitting.current = false;
      if (mounted.current) setSaving(false);
    }
  };
  return (
    <Modal
      title={t('生成视频')}
      visible
      onCancel={() => !saving && onClose()}
      maskClosable={!saving}
      closable={!saving}
      closeOnEsc={!saving}
      footer={null}
      style={{ width: 640, maxWidth: 'calc(100vw - 24px)' }}
    >
      {tokenError && <Banner type='danger' description={tokenError} />}
      {modelError && <Banner type='danger' description={modelError} />}
      <Form
        getFormApi={(api) => (formApiRef.current = api)}
        initValues={{
          duration: 5,
          size: '768P',
          ratio: 'auto',
        }}
        onSubmit={submit}
        disabled={saving}
      >
        <Form.Select
          field='token_id'
          label={t('令牌')}
          optionList={tokens}
          loading={loadingTokens}
          filter
          onChange={(value) => {
            const tokenId = Number(value);
            setSelectedTokenId(tokenId > 0 ? tokenId : null);
            formApiRef.current?.setValue('model', undefined);
            if (tokenId > 0) loadModels(tokenId);
            else {
              ++modelRequestId.current;
              setModels([]);
              setModelError('');
            }
          }}
          style={{ width: '100%' }}
        />
        {!loadingTokens &&
          !tokenError &&
          !tokens.some((token) => !token.disabled) && (
            <Banner type='warning' description={t('暂无可用令牌')} />
          )}
        {(hasMore || loadError) && (
          <Button
            htmlType='button'
            size='small'
            disabled={saving || loadingModels}
            loading={loadingTokens}
            onClick={() => {
              loadTokens(loadError ? 1 : tokenPage + 1);
              if (loadError && selectedTokenId) loadModels(selectedTokenId);
            }}
          >
            {loadError ? t('重试') : t('加载更多')}
          </Button>
        )}
        <Form.Select
          field='model'
          label={t('模型')}
          loading={loadingModels}
          optionList={models.map((model) => ({ label: model, value: model }))}
          style={{ width: '100%' }}
        />
        {!selectedTokenId && !loadingModels && !modelError && (
          <Banner
            type='warning'
            description={t('请先选择令牌')}
          />
        )}
        {selectedTokenId && !loadingModels && !modelError && !models.length && (
          <Banner description={t('暂无可用视频模型')} type='warning' />
        )}
        <Form.TextArea
          field='prompt'
          label={t('提示词')}
          autosize={{ minRows: 3, maxRows: 6 }}
        />
        <div className='grid grid-cols-1 sm:grid-cols-3 gap-x-3'>
          <Form.InputNumber
            field='duration'
            label={t('时长（秒）')}
            min={4}
            max={15}
            style={{ width: '100%' }}
          />
          <Form.Select
            field='size'
            label={t('分辨率')}
            optionList={['768P', '2K'].map((value) => ({
              label: value,
              value,
            }))}
            style={{ width: '100%' }}
          />
          <Form.Select
            field='ratio'
            label={t('画面比例')}
            optionList={[
              { value: 'auto', label: t('自动') },
              ...H3_RATIOS.map((value) => ({
                value,
                label: value === 'adaptive' ? t('自适应') : value,
              })),
            ]}
            style={{ width: '100%' }}
          />
        </div>
        <div className='space-y-2'>
          {media.map((item) => (
            <div
              key={item.id}
              className='grid grid-cols-[minmax(0,1fr)_32px] sm:grid-cols-[140px_minmax(0,1fr)_32px] gap-2'
            >
              <Select
                aria-label={t('素材类型')}
                disabled={saving}
                value={item.role}
                optionList={Object.entries(mediaRoles).map(
                  ([value, label]) => ({ value, label }),
                )}
                onChange={(v) => updateMedia(item.id, 'role', v)}
              />
              <Input
                className='row-start-2 sm:row-start-auto'
                aria-label={t('素材 URL')}
                disabled={saving}
                value={item.url}
                placeholder='https://'
                onChange={(v) => updateMedia(item.id, 'url', v)}
              />
              <Tooltip content={t('删除素材')}>
                <Button
                  className='col-start-2 row-start-1 sm:col-start-3'
                  htmlType='button'
                  aria-label={t('删除素材')}
                  disabled={saving}
                  icon={<Trash2 size={16} />}
                  theme='borderless'
                  onClick={() =>
                    setMedia((current) =>
                      current.filter((entry) => entry.id !== item.id),
                    )
                  }
                />
              </Tooltip>
            </div>
          ))}
        </div>
        <Button
          htmlType='button'
          disabled={saving || media.length >= 15}
          icon={<Plus size={16} />}
          theme='borderless'
          onClick={() =>
            setMedia((current) => [
              ...current,
              { id: nextMediaId.current++, role: 'reference_image', url: '' },
            ])
          }
        >
          {t('添加素材')}
        </Button>
        {submitError && <Banner type='danger' description={submitError} />}
        <div className='flex justify-end gap-2 mt-4'>
          <Button htmlType='button' disabled={saving} onClick={onClose}>
            {t('取消')}
          </Button>
          <Button
            type='primary'
            theme='solid'
            htmlType='submit'
            loading={saving}
            disabled={
              loadingModels ||
              loadingTokens ||
              Boolean(loadError) ||
              !models.length ||
              !tokens.some((token) => !token.disabled)
            }
            icon={<Video size={16} />}
          >
            {t('提交任务')}
          </Button>
        </div>
      </Form>
    </Modal>
  );
};
export default H3GenerationModal;
