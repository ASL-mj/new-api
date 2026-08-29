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

import React, { useEffect, useMemo, useState } from 'react';
import { Modal, Tooltip } from '@douyinfe/semi-ui';
import {
  IconCopy,
  IconChevronDown,
  IconChevronRight,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { copy, showSuccess } from '../../../../helpers';
import { buildUsageLogDetail } from './usageLogDetailAdapter';
import './usageLogDetailModal.css';

const formatCount = (value) => Number(value || 0).toLocaleString();

const UsageLogDetailModal = ({
  showLogDetail,
  closeLogDetail,
  selectedLog,
  expandData,
}) => {
  const { t } = useTranslation();
  const [nativeOpen, setNativeOpen] = useState(false);
  const [extrasOpen, setExtrasOpen] = useState(false);
  const [lastKey, setLastKey] = useState(selectedLog?.key);
  const selectedLogKey = selectedLog?.key;

  const detail = useMemo(() => {
    if (!selectedLog) {
      return null;
    }
    return buildUsageLogDetail({
      record: selectedLog,
      expandRows: expandData?.[selectedLog.key],
      t,
    });
  }, [selectedLog, expandData, t]);

  // 切换到另一条日志时在提交后重置折叠状态，避免渲染阶段更新状态。
  useEffect(() => {
    if (selectedLogKey === lastKey) {
      return;
    }
    setLastKey(selectedLogKey);
    setNativeOpen(false);
    setExtrasOpen(false);
  }, [lastKey, selectedLogKey]);

  const handleCopy = async (event, text) => {
    event.stopPropagation();
    if (await copy(text)) {
      showSuccess(t('已复制'));
    }
  };

  const tokens = detail?.tokens || {};

  return (
    <Modal
      visible={showLogDetail}
      onCancel={closeLogDetail}
      footer={null}
      centered
      maskClosable
      closable={false}
      width={860}
      className='uldm-modal'
      bodyStyle={{ padding: 0 }}
    >
      {detail && (
        <div className='uldm-dialog'>
          <header className='uldm-header'>
            <div className='uldm-meta'>
              <span>
                LOG / {String(detail.id ?? '').padStart(6, '0') || '-'}
              </span>
              <span className='uldm-consume-tag'>{detail.typeLabel}</span>
            </div>
            <h2 className='uldm-title'>{t('消耗详情')}</h2>
            <button
              className='uldm-close'
              aria-label={t('关闭')}
              onClick={closeLogDetail}
            >
              ×
            </button>
          </header>

          <div className='uldm-body'>
            {/* 请求概览：六个字段固定为三行两列，请求 ID 与分组同一行 */}
            <section className='uldm-overview'>
              <div className='uldm-kv'>
                <span className='uldm-kv-label'>{t('请求 ID')}</span>
                <span className='uldm-kv-value uldm-mono'>
                  {detail.requestId ? (
                    <span
                      title={detail.requestId}
                      onClick={(event) => handleCopy(event, detail.requestId)}
                      style={{ cursor: 'pointer' }}
                    >
                      {detail.requestId}
                    </span>
                  ) : (
                    '-'
                  )}
                </span>
              </div>
              <div className='uldm-kv'>
                <span className='uldm-kv-label'>{t('分组')}</span>
                <span className='uldm-kv-value uldm-mono'>{detail.group}</span>
              </div>
              <div className='uldm-kv'>
                <span className='uldm-kv-label'>{t('令牌')}</span>
                <span className='uldm-kv-value uldm-mono'>
                  {detail.tokenName}
                </span>
              </div>
              <div className='uldm-kv'>
                <span className='uldm-kv-label'>{t('调用模型')}</span>
                <span className='uldm-kv-value uldm-mono'>
                  {detail.modelName}
                  {detail.upstreamModelName && (
                    <Tooltip
                      content={`${t('实际模型')}：${detail.upstreamModelName}`}
                    >
                      <span
                        className='uldm-accent'
                        style={{ marginLeft: 6, fontSize: 11 }}
                      >
                        → {detail.upstreamModelName}
                      </span>
                    </Tooltip>
                  )}
                </span>
              </div>
              <div className='uldm-kv'>
                <span className='uldm-kv-label'>{t('推理强度')}</span>
                <span
                  className={`uldm-kv-value ${detail.reasoningEffort ? 'uldm-accent' : ''}`}
                >
                  {detail.reasoningEffort || '-'}
                </span>
              </div>
              <div className='uldm-kv'>
                <span className='uldm-kv-label'>{t('响应时间')}</span>
                <span className='uldm-kv-value uldm-success uldm-mono'>
                  {detail.useTime.toFixed(1)}s
                  {detail.frtSeconds != null && (
                    <small className='uldm-frt'>
                      {' '}
                      (FRT: {detail.frtSeconds}s)
                    </small>
                  )}
                  {detail.speed > 0 && (
                    <small className='uldm-frt'>
                      {' '}
                      · {detail.isStream ? t('流') : t('非流')} {detail.speed}{' '}
                      t/s
                    </small>
                  )}
                </span>
              </div>
            </section>

            {/* 消耗明细：输入 / 输出 / 缓存读取 / 总 Token 一行四项 */}
            <section className='uldm-section'>
              <div className='uldm-section-head'>
                <h3 className='uldm-section-title'>{t('消耗明细')}</h3>
                <div className='uldm-section-tools'>
                  {detail.requestPath && (
                    <>
                      <span>{t('路径')}</span>
                      <code className='uldm-path'>{detail.requestPath}</code>
                      <Tooltip content={t('复制路径')}>
                        <button
                          className='uldm-copy-btn'
                          aria-label={t('复制路径')}
                          onClick={(event) =>
                            handleCopy(event, detail.requestPath)
                          }
                        >
                          <IconCopy size='small' />
                        </button>
                      </Tooltip>
                    </>
                  )}
                  {detail.nativeContent && (
                    <button
                      className='uldm-link-btn'
                      onClick={() => setNativeOpen((open) => !open)}
                    >
                      {t('原生格式')}
                      {nativeOpen ? (
                        <IconChevronDown size='small' />
                      ) : (
                        <IconChevronRight size='small' />
                      )}
                    </button>
                  )}
                </div>
              </div>
              <div className='uldm-token-grid'>
                <div className='uldm-token-cell'>
                  <span className='uldm-token-label'>{t('输入')}</span>
                  <strong className='uldm-token-value'>
                    {formatCount(tokens.input)}
                  </strong>
                </div>
                <div className='uldm-token-cell'>
                  <span className='uldm-token-label'>{t('输出')}</span>
                  <strong className='uldm-token-value'>
                    {formatCount(tokens.output)}
                  </strong>
                </div>
                <div className='uldm-token-cell'>
                  <span className='uldm-token-label'>
                    {t('缓存读取')}{' '}
                    <em className='uldm-hit-rate'>
                      {(tokens.hitRate || 0).toFixed(2)}%
                    </em>
                  </span>
                  <strong className='uldm-token-value'>
                    {formatCount(tokens.cacheRead)}
                  </strong>
                </div>
                <div className='uldm-token-cell'>
                  <span className='uldm-token-label'>{t('总 Token')}</span>
                  <strong className='uldm-token-value'>
                    {formatCount(tokens.total)}
                  </strong>
                </div>
              </div>
              {nativeOpen && detail.nativeContent && (
                <div className='uldm-process uldm-native'>
                  {detail.nativeContent}
                </div>
              )}
            </section>

            {/* 计费详情 */}
            <section className='uldm-section'>
              <div className='uldm-section-head'>
                <h3 className='uldm-section-title'>{t('计费详情')}</h3>
                {detail.billing && (
                  <div className='uldm-billing-meta'>
                    <span>
                      {t('模式')}
                      <strong>{detail.billing.modeLabel}</strong>
                    </span>
                    {detail.billing.tierLabel && (
                      <span>
                        {t('命中阶梯')}
                        <strong>{detail.billing.tierLabel}</strong>
                      </span>
                    )}
                  </div>
                )}
              </div>
              {detail.billing ? (
                <>
                  {detail.billing.items.map((item, index) => {
                    const formula = item.isPerRequest
                      ? `1 × ${t('按次')} = ${item.amount}`
                      : `${formatCount(item.tokens)} Token × ${t('{{price}}/1M Token', { price: item.unitPriceCompact })} = ${item.amount}`;
                    return (
                      <div className='uldm-billing-item' key={index}>
                        <strong className='uldm-billing-label'>
                          {item.label}
                        </strong>
                        <span className='uldm-formula'>{formula}</span>
                        <strong className='uldm-amount'>{item.amount}</strong>
                      </div>
                    );
                  })}
                  <div className='uldm-billing-total'>
                    <div className='uldm-subtotal'>
                      <span className='uldm-total-label'>{t('分项合计')}</span>
                      <strong className='uldm-total-equation'>
                        {detail.billing.subtotalText}{' '}
                        <span className='uldm-ratio'>
                          × {detail.billing.multiplierText}
                        </span>{' '}
                        = {detail.billing.finalText}
                      </strong>
                      <span className='uldm-total-label'>
                        {detail.billing.multiplierLabel}
                      </span>
                    </div>
                    <div className='uldm-final'>
                      <span className='uldm-total-label'>{t('最终扣费')}</span>
                      <strong className='uldm-total-equation'>
                        {detail.billing.finalText}
                      </strong>
                    </div>
                  </div>
                </>
              ) : (
                <div className='uldm-process'>
                  {detail.violation ? (
                    <div style={{ marginBottom: 8 }}>
                      <span className='uldm-violation'>{t('违规扣费')}</span>
                      <span className='uldm-mono' style={{ marginLeft: 8 }}>
                        {t('扣费')}：{detail.violation.feeText}
                      </span>
                    </div>
                  ) : null}
                  {detail.billingProcess
                    ? detail.billingProcess
                    : detail.contentText || t('暂无计费过程数据')}
                </div>
              )}
            </section>

            {/* 其他扩展信息：默认折叠，点击展开 */}
            {detail.extraRows.length > 0 && (
              <section className='uldm-section'>
                <button
                  type='button'
                  className='uldm-collapse-head'
                  onClick={() => setExtrasOpen((open) => !open)}
                >
                  <span className='uldm-section-title'>{t('扩展信息')}</span>
                  <span className='uldm-collapse-meta'>
                    {detail.extraRows.length}
                    {extrasOpen ? (
                      <IconChevronDown size='small' />
                    ) : (
                      <IconChevronRight size='small' />
                    )}
                  </span>
                </button>
                {extrasOpen && (
                  <div className='uldm-extras'>
                    {detail.extraRows.map((row, index) => (
                      <div
                        className='uldm-extra-row'
                        key={`${row.key}-${index}`}
                      >
                        <span className='uldm-extra-label'>{row.key}</span>
                        <span className='uldm-extra-value'>{row.value}</span>
                      </div>
                    ))}
                  </div>
                )}
              </section>
            )}
          </div>
        </div>
      )}
    </Modal>
  );
};

export default UsageLogDetailModal;
