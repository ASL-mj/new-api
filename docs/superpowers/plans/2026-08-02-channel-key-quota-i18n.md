# Multi-Key Quota Amount and Custom I18n Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the multi-key quota amount/native-unit mismatch and internationalize all custom NewAPI features in the supported frontend and backend languages.

**Architecture:** Keep integer quota units as the only database and API representation, and convert only at the frontend form/render boundary. Use the existing i18next Chinese-source-key convention for UI copy, typed localization metadata for backend business errors, and a `message_key` field for request-locale rendering of new system events while preserving historical event text.

**Tech Stack:** Go, Gin, GORM, go-i18n, React 18, Semi UI, i18next, Bun tests, MySQL/SQLite test paths.

---

## File Map

- `web/src/helpers/channelQuota.js`: pure multi-key quota editor and display helpers.
- `web/src/helpers/channelQuota.test.js`: currency/native-unit regression tests.
- `web/src/components/table/channels/modals/MultiKeyManageModal.jsx`: amount-first key limit editor and localized labels.
- `web/src/components/table/channels/modals/EditChannelModal.jsx`: localized amount display for current used quota.
- `web/src/components/table/channels/renderers/ChannelUsageCells.jsx`: translated balance tooltip.
- `web/src/components/monitor-status/*`: translated refresh, provider, status, and empty-state copy.
- `web/src/pages/Ops/**`, `web/src/hooks/ops/useOpsData.jsx`: translated operations labels, descriptions, wave labels, and dynamic period keys.
- `web/src/i18n/locales/*.json`: complete custom keys in seven frontend languages.
- `web/src/i18n/customKeys.js`: canonical list of translation keys introduced by the Skye customization.
- `web/src/i18n/customI18n.test.js`: custom-key parity, empty-value, and interpolation-token checks.
- `common/localized_error.go`: typed backend localization error without importing the i18n package.
- `common/gin.go`: translate typed errors in the existing API response boundary.
- `i18n/keys.go`, `i18n/locales/*.yaml`: custom backend response and event keys in three languages.
- `controller/channel_usage.go`, `controller/monitor_group.go`, `controller/monitor_status.go`, `controller/ops.go`, `controller/channel.go`: localized custom API responses.
- `model/system_event_log.go`: optional `message_key` persistence.
- `controller/system_event_log.go`: request-locale rendering for structured events.
- `controller/monitor_group_runner.go`, `controller/relay.go`, `pkg/ops_metrics/events.go`, `pkg/perf_metrics/events.go`, `service/channel.go`, `service/channel_usage.go`: structured event keys and arguments.
- Relevant `*_test.go` files: typed error, API translation, quota/token separation, event localization, and migration coverage.

### Task 1: Lock the Quota Unit Boundary With Failing Tests

**Files:**
- Create: `web/src/helpers/channelQuota.test.js`
- Modify: `service/channel_usage_test.go`

- [ ] **Step 1: Add frontend amount/native-unit tests**

Create a localStorage stub and assert the intended boundary:

```js
globalThis.localStorage = createStorage({
  quota_display_type: 'USD',
  quota_per_unit: '500000',
});

expect(createKeyQuotaDraft(500000)).toEqual({
  quota: 500000,
  amount: 1,
});
expect(keyQuotaFromAmount(1)).toEqual({
  quota: 500000,
  amount: 1,
});
expect(formatKeyQuotaUsage(250000, 500000)).toEqual({
  used: '$0.50',
  limit: '$1.00',
});
```

Also cover CNY, custom currency, TOKENS, invalid values, and zero-as-unlimited.

- [ ] **Step 2: Run the frontend test and confirm it fails**

Run:

```bash
cd web && bun test src/helpers/channelQuota.test.js
```

Expected: failure because `channelQuota.js` and its exports do not exist.

- [ ] **Step 3: Add a backend regression proving TokenUsed cannot consume quota**

Add a test that records `Quota: 10` and `TokenUsed: 100000`, then verifies:

```go
assert.EqualValues(t, 10, keyUsage.QuotaLimitUsed)
assert.EqualValues(t, 10, daily.Quota)
assert.EqualValues(t, 100000, daily.TokenUsed)
assert.Equal(t, common.ChannelStatusEnabled, keyUsage.Status)
```

- [ ] **Step 4: Run the targeted backend test**

Run:

```bash
go test ./service -run 'TestRecordChannelUsage.*Token' -count=1
```

Expected: pass against the current backend, documenting that the root cause is not the settlement layer.

### Task 2: Implement Amount-First Multi-Key Quota Editing

**Files:**
- Create: `web/src/helpers/channelQuota.js`
- Modify: `web/src/components/table/channels/modals/MultiKeyManageModal.jsx`
- Modify: `web/src/components/table/channels/modals/EditChannelModal.jsx`

- [ ] **Step 1: Implement pure quota editor helpers**

Use the existing conversion and render helpers:

```js
import { renderQuota } from './render';
import { displayAmountToQuota, quotaToDisplayAmount } from './quota';

export const normalizeNativeQuota = (value) =>
  Math.max(0, Math.round(Number(value) || 0));

export const createKeyQuotaDraft = (quota) => {
  const normalized = normalizeNativeQuota(quota);
  return {
    quota: normalized,
    amount: Number(quotaToDisplayAmount(normalized).toFixed(6)),
  };
};

export const keyQuotaFromAmount = (amount) => {
  const normalizedAmount = Math.max(0, Number(amount) || 0);
  return {
    amount: normalizedAmount,
    quota: normalizeNativeQuota(displayAmountToQuota(normalizedAmount)),
  };
};

export const keyQuotaFromNative = (quota) => createKeyQuotaDraft(quota);

export const formatKeyQuotaUsage = (used, limit) => ({
  used: renderQuota(normalizeNativeQuota(used)),
  limit: normalizeNativeQuota(limit) > 0 ? renderQuota(normalizeNativeQuota(limit)) : '∞',
});
```

- [ ] **Step 2: Replace the raw integer modal with an amount-first form**

In `handleEditKeyQuota`:

- initialize `{ amount, quota }` with `createKeyQuotaDraft`;
- show the current currency symbol on the amount input;
- provide a collapsed native-quota input;
- synchronize both values through the helper functions;
- submit only `{ quota_limit: draft.quota }`;
- keep `0` as unlimited.

- [ ] **Step 3: Render key usage in the selected currency**

Replace raw `{used} / {limit}` output with `formatKeyQuotaUsage(used, limit)`. Keep percentage calculation based on raw integers.

- [ ] **Step 4: Render channel current usage consistently**

Replace `inputs.quota_limit_used || 0` in `EditChannelModal.jsx` with `renderQuota(inputs.quota_limit_used || 0)`.

- [ ] **Step 5: Run quota helper tests**

Run:

```bash
cd web && bun test src/helpers/channelQuota.test.js
```

Expected: all tests pass.

### Task 3: Convert Remaining Custom Frontend Copy to i18next

**Files:**
- Modify: custom files under `web/src/components/monitor-status/`
- Modify: custom files under `web/src/components/table/monitor-groups/`
- Modify: custom files under `web/src/pages/GroupStatus/`
- Modify: custom files under `web/src/pages/Ops/`
- Modify: `web/src/hooks/ops/useOpsData.jsx`
- Modify: `web/src/components/table/channels/renderers/ChannelUsageCells.jsx`
- Modify: `web/src/components/settings/OtherSetting.jsx`

- [ ] **Step 1: Remove visible hardcoded Chinese strings**

Apply these patterns:

```jsx
<Tooltip content={t('点击更新上游余额')}>
```

```jsx
const formatMonitorRefresh = (seconds, t) =>
  t('{{seconds}} s 后刷新', { seconds: Math.max(0, Number(seconds) || 0) });
```

```jsx
const metric = {
  title: t('所有请求'),
  description: t('统计选定时间范围内的全部请求、Token、平均 QPS 与 TPS。'),
};
```

Status and period metadata may retain stable machine values, but every displayed label must pass through `t`.

- [ ] **Step 2: Add explicit dynamic-key registration**

Ensure dynamic keys such as `正常`, `降级`, `故障`, `超时`, `暂无数据`, `近 1 小时`, `近 6 小时`, `近 24 小时`, `近 7 天`, and `近 30 天` exist in every locale even when the extractor cannot see `t(option.label)`.

- [ ] **Step 3: Run extraction status before adding translations**

Run:

```bash
cd web && npm run i18n:status
```

Expected: the command reports missing custom keys, which becomes the translation checklist.

### Task 4: Complete Seven Frontend Locale Files

**Files:**
- Modify: `web/src/i18n/locales/zh-CN.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/vi.json`
- Create: `web/src/i18n/customKeys.js`
- Create: `web/src/i18n/customI18n.test.js`

- [ ] **Step 1: Add every custom source key to zh-CN**

Use the Chinese key as the Simplified Chinese value. Include all custom modules from the baseline diff, not only strings touched by the quota fix.

- [ ] **Step 2: Translate the same key set in the other six locales**

Preserve technical tokens such as `TPS`, `TTFT`, `QPS`, `SLA`, `Ping`, `Token`, API paths, model names, and interpolation placeholders.

- [ ] **Step 3: Add the canonical custom-key list and locale parity tests**

`customKeys.js` must list the keys introduced by the customization. The test loads all seven JSON files and verifies only that custom set, without requiring unrelated upstream locale files to have identical historical key sets:

```js
for (const key of SKYE_CUSTOM_I18N_KEYS) {
  expect(locale.translation[key]).toBeDefined();
  expect(locale.translation[key].trim()).not.toBe('');
  expect(extractInterpolations(locale.translation[key])).toEqual(
    extractInterpolations(key),
  );
}
```

Limit interpolation comparison to keys containing `{{...}}`; allow translated values to retain technical terms.

- [ ] **Step 4: Run frontend i18n tests and lint**

Run:

```bash
cd web && bun test src/i18n/customI18n.test.js
cd web && npm run i18n:lint
```

Expected: no missing keys, empty values, or interpolation mismatches.

### Task 5: Add Typed Backend Business-Error Localization

**Files:**
- Create: `common/localized_error.go`
- Create: `common/localized_error_test.go`
- Modify: `common/gin.go`
- Modify: `i18n/keys.go`
- Modify: `i18n/locales/zh-CN.yaml`
- Modify: `i18n/locales/zh-TW.yaml`
- Modify: `i18n/locales/en.yaml`

- [ ] **Step 1: Write typed-error tests**

Cover key, template args, wrapped cause, and `errors.As`:

```go
err := NewLocalizedError("channel_usage.max_batch", map[string]any{"Max": 200})
var localized *LocalizedError
require.ErrorAs(t, err, &localized)
assert.Equal(t, "channel_usage.max_batch", localized.Key)
```

- [ ] **Step 2: Implement `LocalizedError`**

The type lives in `common`, stores `Key`, `Args`, and optional `Cause`, and does not import the `i18n` package to avoid a cycle.

- [ ] **Step 3: Teach `ApiError` to translate typed errors**

Before returning `err.Error()`, use `errors.As` and `TranslateMessage(c, localized.Key, localized.Args)`.

- [ ] **Step 4: Add custom backend keys and three-language messages**

Create namespaced keys for:

- `channel_quota.*`
- `channel_usage.*`
- `monitor_group.*`
- `monitor_status.*`
- `ops.*`
- `system_event.*`

Use Go template placeholders matching the `go-i18n` convention already used in the YAML files.

- [ ] **Step 5: Run common and i18n tests**

Run:

```bash
go test ./common ./i18n -count=1
```

Expected: all tests pass.

### Task 6: Localize Custom API Responses

**Files:**
- Modify: `controller/channel_usage.go`
- Modify: `controller/channel.go`
- Modify: `controller/monitor_group.go`
- Modify: `controller/monitor_status.go`
- Modify: `controller/ops.go`
- Modify: relevant controller tests

- [ ] **Step 1: Add language-response tests**

For each custom controller family, send requests with `Accept-Language: en`, `zh-CN`, and `zh-TW`, and verify at least one success and one validation failure message.

- [ ] **Step 2: Replace custom success strings**

Use `common.ApiSuccessI18n` for quota reset/update and monitor run/create/update operations.

- [ ] **Step 3: Replace predictable validation strings**

Return `common.NewLocalizedError` from validation helpers or call `common.ApiErrorI18n` directly at the response boundary. Preserve unexpected database errors as internal errors rather than incorrectly translating them as validation failures.

- [ ] **Step 4: Run targeted controller tests**

Run:

```bash
go test ./controller -run 'Test(ChannelUsage|MonitorGroup|MonitorStatus|Ops).*I18n' -count=1
```

Expected: all three languages pass.

### Task 7: Localize New System Events Without Rewriting History

**Files:**
- Modify: `model/system_event_log.go`
- Modify: `controller/system_event_log.go`
- Modify: `controller/monitor_group_runner.go`
- Modify: `controller/relay.go`
- Modify: `pkg/ops_metrics/events.go`
- Modify: `pkg/perf_metrics/events.go`
- Modify: `service/channel.go`
- Modify: `service/channel_usage.go`
- Modify: relevant model, controller, package, and service tests

- [ ] **Step 1: Add migration and response tests**

Verify `message_key` is migrated, a structured event is translated according to `Accept-Language`, and an old event with an empty key returns its original `message` unchanged.

- [ ] **Step 2: Add the optional model field**

```go
MessageKey string `json:"message_key,omitempty" gorm:"size:128;index"`
```

Continue storing readable fallback text in `Message`; use the existing `Extra` JSON object as template data.

- [ ] **Step 3: Localize rows at the API boundary**

In `GetSystemEventLogs`, parse `Extra` into a map, add standard row fields where needed, and replace only the response copy of `Message` with `i18n.T(c, row.MessageKey, args)`.

- [ ] **Step 4: Add message keys to every custom event writer**

Cover monitor probe lifecycle, channel auto-disable/restore, channel and key quota exhaustion, operations flush recovery/failure, performance flush recovery/failure, and final upstream relay failure.

- [ ] **Step 5: Run event tests**

Run:

```bash
go test ./model ./service ./controller ./pkg/ops_metrics ./pkg/perf_metrics -run 'SystemEvent|EventLocalization|QuotaExhausted' -count=1
```

Expected: structured new events translate and legacy rows remain unchanged.

### Task 8: Full Verification and Cleanup

**Files:**
- Modify only files required by failures found during verification.

- [ ] **Step 1: Format changed files**

Run `gofmt` on changed Go files and Prettier on changed frontend files.

- [ ] **Step 2: Run frontend tests**

```bash
cd web && bun test src/helpers/channelQuota.test.js src/i18n/customI18n.test.js src/helpers/performance.test.js src/components/monitor-status/monitorTimelineUtils.test.js
```

- [ ] **Step 3: Run frontend i18n checks and production build**

```bash
cd web && npm run i18n:status
cd web && npm run i18n:lint
cd web && npm run build
```

- [ ] **Step 4: Run backend tests**

```bash
go test ./common ./i18n ./model ./service ./controller ./pkg/ops_metrics ./pkg/perf_metrics -count=1
go test ./... -count=1
```

- [ ] **Step 5: Run static diff audits**

Verify:

- no newly added visible Chinese strings remain outside `t(...)` or translation resources;
- all seven frontend locales contain the custom key set;
- all three backend locale files contain every new key constant;
- `git diff --check` passes;
- `bin/new-api-v0132-local` remains untracked and unstaged.

- [ ] **Step 6: Smoke-test the running application**

Restart the local backend and frontend, then verify:

- editing a key with `$1` sends `quota_limit=QuotaPerUnit`;
- key used/limit values switch with the selected currency;
- switching the UI among English, Traditional Chinese, and one non-CJK language updates every custom page;
- new system events change language when queried under another administrator language;
- old system events retain their original text.
