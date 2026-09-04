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

const AdminQuotaDetail = ({ detail, t, onCopy }) => (
  <div className='uldm-management-body'>
    <div className='uldm-management-rule' />
    <section className='uldm-management-content'>
      <h3 className='uldm-management-content-title'>{t('内容')}</h3>
      <div className='uldm-management-content-value'>
        <span>{detail.contentText || '-'}</span>
        {detail.contentText && (
          <Tooltip content={t('复制')}>
            <button
              type='button'
              className='uldm-copy-btn'
              aria-label={t('复制')}
              onClick={(event) => onCopy(event, detail.contentText)}
            >
              <IconCopy size='small' />
            </button>
          </Tooltip>
        )}
      </div>
    </section>
  </div>
);

const DetailOverview = ({ items, onCopy }) => {
  const requestIdOwnRow =
    items.length % 2 === 1 && items.some((item) => item.requestId);

  return (
    <section
      className={`uldm-overview ${requestIdOwnRow ? 'uldm-overview--request-id-full' : ''}`}
    >
      {items.map((item) => (
        <div
          className={`uldm-kv ${requestIdOwnRow && item.requestId ? 'uldm-kv--full' : ''}`}
          key={item.label}
        >
          <span className='uldm-kv-label'>{item.label}</span>
          <span
            className={`uldm-kv-value ${item.mono ? 'uldm-mono' : ''} ${item.tone === 'success' ? 'uldm-success' : ''}`}
          >
            {item.copyable ? (
              <button
                type='button'
                className='uldm-inline-copy'
                onClick={(event) => onCopy(event, item.value)}
                title={item.value}
              >
                {item.value}
              </button>
            ) : (
              item.value
            )}
            {item.subValue && (
              <small className='uldm-sub-value'>{item.subValue}</small>
            )}
          </span>
        </div>
      ))}
    </section>
  );
};

const ErrorDetail = ({ error, t }) => {
  const rows = [
    [t('HTTP 状态码'), error.statusCode],
    [t('错误码'), error.errorCode],
    [t('错误类型'), error.errorType],
    [t('链路阶段'), error.stage],
    [t('传输错误'), error.transportError],
  ].filter(([, value]) => value !== undefined && value !== null && value !== '');

  return (
    <section className='uldm-section'>
      <div className='uldm-section-head'>
        <h3 className='uldm-section-title'>{t('请求结果')}</h3>
        <strong className='uldm-error'>✗ {t('请求失败')}</strong>
      </div>
      {rows.length > 0 && (
        <div className='uldm-detail-grid'>
          {rows.map(([label, value]) => (
            <div className='uldm-detail-cell' key={label}>
              <span className='uldm-kv-label'>{label}</span>
              <strong className='uldm-detail-value'>{value}</strong>
            </div>
          ))}
        </div>
      )}
      {error.errorMessage && (
        <div className='uldm-process uldm-error-message'>
          <span className='uldm-kv-label'>{t('错误信息')}</span>
          <div>{error.errorMessage}</div>
        </div>
      )}
    </section>
  );
};

const EventDetail = ({ event, t, onCopy }) => (
  <section className='uldm-section'>
    <div className='uldm-section-head'>
      <h3 className='uldm-section-title'>{event.title}</h3>
    </div>
    <div className='uldm-process uldm-event-content'>
      <span className='uldm-kv-label'>{t('操作内容')}</span>
      <div>{event.content}</div>
    </div>
    {event.rows.length > 0 && (
      <div className='uldm-detail-grid'>
        {event.rows.map((row) => (
          <div className='uldm-detail-cell' key={row.label}>
            <span className='uldm-kv-label'>{row.label}</span>
            {row.copyable ? (
              <button
                type='button'
                className={`uldm-inline-copy ${row.mono ? 'uldm-mono' : ''}`}
                onClick={(event) => onCopy(event, row.value)}
                title={row.value}
              >
                {row.value}
              </button>
            ) : (
              <strong className='uldm-detail-value'>{row.value}</strong>
            )}
          </div>
        ))}
      </div>
    )}
  </section>
);

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
            <h2 className='uldm-title'>{detail.title}</h2>
            <button
              className='uldm-close'
              aria-label={t('关闭')}
              onClick={closeLogDetail}
            >
              ×
            </button>
          </header>

          {detail.isAdminQuotaAdjustment ? (
            <AdminQuotaDetail detail={detail} t={t} onCopy={handleCopy} />
          ) : (
            <div className='uldm-body'>
              <DetailOverview
                items={detail.overviewItems}
                onCopy={handleCopy}
              />

              {detail.error && (
                <ErrorDetail error={detail.error} t={t} />
              )}

              {detail.event && (
                <EventDetail event={detail.event} t={t} onCopy={handleCopy} />
              )}

              {detail.probe && (
                <section className='uldm-section'>
                  <div className='uldm-section-head'>
                    <h3 className='uldm-section-title'>{t('探测结果')}</h3>
                    <strong
                      className={
                        detail.probe.success ? 'uldm-success' : 'uldm-error'
                      }
                    >
                      {detail.probe.success
                        ? `✓ ${t('成功')}`
                        : `✗ ${t('失败')}`}
                    </strong>
                  </div>
                  <div className='grid grid-cols-2 gap-3 p-3 sm:grid-cols-4'>
                    {[
                      [t('HTTP 状态码'), detail.probe.statusCode],
                      [t('错误码'), detail.probe.errorCode],
                      [t('错误类型'), detail.probe.errorType],
                      [
                        t('标准口径'),
                        detail.probe.billingScope ? t('是') : '-',
                      ],
                    ].map(([label, value]) => (
                      <div key={label} className='min-w-0'>
                        <span className='uldm-kv-label'>{label}</span>
                        <div className='mt-1 break-words text-sm'>
                          {value ?? '-'}
                        </div>
                      </div>
                    ))}
                  </div>
                  {detail.probe.errorMessage && (
                    <div className='border-t p-3'>
                      <span className='uldm-kv-label'>{t('错误信息')}</span>
                      <div className='mt-1 whitespace-pre-wrap break-words text-sm'>
                        {detail.probe.errorMessage}
                      </div>
                    </div>
                  )}
                </section>
              )}

              {detail.showUsage && (
                <section className='uldm-section'>
                <div className='uldm-section-head'>
                  <h3 className='uldm-section-title'>{detail.usageTitle}</h3>
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
              )}

              {detail.showBilling && (
                <section className='uldm-section'>
                <div className='uldm-section-head'>
                  <h3 className='uldm-section-title'>{detail.billingTitle}</h3>
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
                        <span className='uldm-total-label'>
                          {t('分项合计')}
                        </span>
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
                        <span className='uldm-total-label'>
                          {detail.kind === 'probe'
                            ? t('标准口径消耗')
                            : t('最终扣费')}
                        </span>
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
              )}

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
          )}
        </div>
      )}
    </Modal>
  );
};

export default UsageLogDetailModal;
