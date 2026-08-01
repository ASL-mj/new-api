# Channel Quota, Usage, and User Rankings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add reliable channel and per-key quota enforcement, channel usage columns, and switchable user quota/token/request rankings to NewAPI v0.13.2.

**Architecture:** Route every channel billing settlement through one atomic usage service that updates channel totals, stable per-key usage records, daily aggregates, status, abilities, and cache. Keep the dashboard ranking change independent and reuse the existing `/api/data/users` payload.

**Tech Stack:** Go, Gin, GORM, MySQL/SQLite/PostgreSQL, React, Semi UI, VChart, Vitest/Jest-compatible frontend tests.

---

## File Map

**Create**

- `model/channel_usage.go`: channel quota settlement result, per-key usage model, daily aggregate model, batch stats queries, reset helpers.
- `model/channel_usage_test.go`: isolated database tests for atomic increments, resets, daily upserts, and multi-key identity.
- `service/channel_usage.go`: billing-path orchestration, automatic disable events, cache and ability synchronization.
- `service/channel_usage_test.go`: service-level first-crossing and status transition tests.
- `controller/channel_usage.go`: batch stats, key usage, reset, and key-limit endpoints.
- `controller/channel_usage_test.go`: request validation and response contract tests.
- `web/src/hooks/channels/useChannelUsageStats.js`: page-level batch loading with request sequencing.
- `web/src/components/table/channels/renderers/ChannelUsageCells.jsx`: the three compact usage renderers.
- `web/src/components/table/channels/modals/ChannelQuotaResetModal.jsx`: explicit reset confirmation.
- `web/src/components/table/channels/__tests__/channelUsage.test.jsx`: list and stale-response behavior tests.

**Modify**

- `model/channel.go`: quota fields, schedulability helpers, enable guards, stable multi-key quota filtering.
- `model/main.go`: auto-migrate the two new tables.
- `model/channel_cache.go`: ensure immediate local cache removal/update when quota is exhausted.
- `service/quota.go`, `service/text_quota.go`, `service/task_billing.go`, `service/violation_fee.go`, `relay/mjproxy_handler.go`: replace direct channel quota increments with the unified service.
- `relay/common/relay_info.go`: carry selected key index and enough settlement identity to the usage service.
- `controller/channel.go`: accept quota settings and enforce reset-before-enable.
- `router/api-router.go`: register new admin routes.
- `web/src/components/table/channels/ChannelsColumnDefs.jsx`: replace current usage columns with the three agreed columns.
- `web/src/components/table/channels/modals/EditChannelModal.jsx`: add quota mode and channel limit fields.
- `web/src/components/table/channels/modals/MultiKeyManageModal.jsx`: show/edit/reset per-key quotas.
- `web/src/components/table/channels/index.jsx` or the owning channel table hook: load batch stats for visible rows.
- `web/src/helpers/dashboard.jsx`: aggregate user quota, tokens, and requests.
- `web/src/hooks/dashboard/useDashboardCharts.jsx`: generate ranking data for the active metric.
- `web/src/components/dashboard/ChartsPanel.jsx`: add the metric switch.
- locale files under `web/src/locales` or the repository's existing i18n path.

## Task 1: Lock Quota Semantics in Model Tests

- [ ] Add failing tests in `model/channel_usage_test.go` for `none`, `channel`, `key`, and `both` mode normalization.
- [ ] Add a failing test proving `quota_limit == 0` is unlimited.
- [ ] Add a failing test proving a channel with `used >= limit` is not quota-schedulable.
- [ ] Add a failing test proving reset clears only `quota_limit_used` and records `quota_limit_reset_at` without changing channel status.
- [ ] Run `go test ./model -run 'TestChannelQuota|TestResetChannelQuota' -count=1` and confirm the tests fail because the fields and helpers do not exist.
- [ ] Add `QuotaLimitMode`, `QuotaLimit`, `QuotaLimitUsed`, and `QuotaLimitResetAt` to `model.Channel`.
- [ ] Implement `NormalizeQuotaLimitMode()`, `UsesChannelQuota()`, `UsesKeyQuota()`, and `IsChannelQuotaExceeded()` in `model/channel.go`.
- [ ] Run the targeted tests and confirm they pass.
- [ ] Commit with `git commit -m "feat: add channel quota model semantics"`.

## Task 2: Add Stable Per-Key Identity and Tables

- [ ] Add failing tests that the same key produces the same fingerprint after key reordering and a different key produces a different fingerprint.
- [ ] Add a failing test that API responses expose only `key_mask` and never the full key.
- [ ] Implement `ChannelKeyUsage` and `ChannelUsageDaily` in `model/channel_usage.go`.
- [ ] Implement `FingerprintChannelKey(key string)` using HMAC-SHA256 and a stable installation secret.
- [ ] Implement `MaskChannelKey(key string)` using the repository's existing key masking convention.
- [ ] Add unique indexes `(channel_id, key_fingerprint)` and `(channel_id, key_fingerprint, usage_date)`.
- [ ] Add both models to `DB.AutoMigrate()` in `model/main.go`.
- [ ] Run `go test ./model -run 'TestChannelKeyFingerprint|TestChannelUsageMigration' -count=1`.
- [ ] Commit with `git commit -m "feat: add channel key and daily usage storage"`.

## Task 3: Implement Atomic Channel Settlement

- [ ] Add failing tests for 10 concurrent increments and assert the final channel `used_quota` and `quota_limit_used` equal the full sum.
- [ ] Add a failing test where two increments cross the limit concurrently and assert only one result has `ChannelJustExhausted=true`.
- [ ] Add a failing test that the crossing request is counted and the channel status becomes auto-disabled.
- [ ] Implement `ApplyChannelUsage(tx, channelID, quota)` with a GORM transaction and arithmetic expressions rather than read-modify-write structs.
- [ ] After the atomic increment, run a second conditional status update with `WHERE status = enabled AND quota_limit > 0 AND quota_limit_used >= quota_limit`.
- [ ] Treat `RowsAffected == 1` from the conditional status update as the only `ChannelJustExhausted=true` result, then re-read the latest values in the same transaction.
- [ ] Keep SQL placeholders and expressions portable across MySQL, SQLite, and PostgreSQL; do not use `RETURNING`, JSONB, or dialect-specific upsert syntax in shared code.
- [ ] Run `go test ./model -run 'TestApplyChannelUsage' -count=1`.
- [ ] Commit with `git commit -m "feat: atomically enforce channel quotas"`.

## Task 4: Implement Atomic Per-Key Settlement

- [ ] Add failing tests for concurrent increments on one key.
- [ ] Add a failing test that only the exhausted key becomes disabled while sibling keys remain enabled.
- [ ] Add a failing test that exhausting the last usable key disables the parent channel.
- [ ] Implement `EnsureChannelKeyUsageRecords(channel)` to reconcile current keys by fingerprint and refresh their display indexes without resetting usage.
- [ ] Implement `ApplyChannelKeyUsage(tx, channel, selectedKey, keyIndex, quota)` with atomic increment and first-crossing detection.
- [ ] Synchronize the stable key status back into `ChannelInfo.MultiKeyStatusList`, `MultiKeyDisabledReason`, and `MultiKeyDisabledTime` for compatibility with existing selection code.
- [ ] Change `GetNextEnabledKey()` to skip a key when its persisted quota is exhausted even if a stale index map says enabled.
- [ ] Run `go test ./model -run 'TestApplyChannelKeyUsage|TestQuotaAwareKeySelection' -count=1`.
- [ ] Commit with `git commit -m "feat: enforce per-key channel quotas"`.

## Task 5: Add Daily Usage Aggregation

- [ ] Add failing tests for channel summary and key detail daily rows.
- [ ] Add a failing test that repeated writes on the same date increment one row instead of inserting duplicates.
- [ ] Add a failing test that the date follows the configured data-export timezone.
- [ ] Implement portable daily upsert using transaction-scoped `UPDATE ...` followed by `CREATE` when `RowsAffected == 0`, retrying the update on unique-race failure.
- [ ] Record quota, token usage, and request count for both `key_fingerprint=''` and the selected key fingerprint.
- [ ] Implement `GetChannelUsageStatsBatch(channelIDs, today, start30d)` returning zero-filled results for every requested channel.
- [ ] Add an index-friendly grouped query over `channel_usage_daily`, never scan raw logs when rendering the list.
- [ ] Run `go test ./model -run 'TestChannelUsageDaily|TestGetChannelUsageStatsBatch' -count=1`.
- [ ] Commit with `git commit -m "feat: aggregate channel daily usage"`.

## Task 6: Route Every Billing Path Through One Service

- [ ] Add a service test that a successful settlement updates history, channel limit, key limit, and daily aggregates once.
- [ ] Add service tests for zero/negative quota and duplicate settlement protection where an existing request-level guard is available.
- [ ] Implement `service.RecordChannelUsage(relayInfo, quota, tokenUsed, requestCount)` in `service/channel_usage.go`.
- [ ] On first channel crossing, write a `SystemEventLog`, update abilities, and immediately update local channel cache.
- [ ] On first key crossing, write a key-specific system event and refresh the channel cache representation.
- [ ] Replace every direct `model.UpdateChannelUsedQuota()` call in text, streaming, task, violation fee, and Midjourney billing paths.
- [ ] Preserve the old `used_quota` behavior for non-limited channels; do not keep the old five-second batch path for limited channels.
- [ ] Run `rg -n "UpdateChannelUsedQuota\(" service relay` and confirm only the compatibility implementation or tests remain.
- [ ] Run `go test ./service ./relay/... -count=1`.
- [ ] Commit with `git commit -m "refactor: centralize channel usage settlement"`.

## Task 7: Enforce Quota During Scheduling and Manual Enable

- [ ] Add failing tests that exhausted channels are excluded in both database and memory-cache selection modes.
- [ ] Add failing controller tests that enabling an exhausted channel or key returns a reset-required message.
- [ ] Add a test that changing the limit to zero does not automatically enable the object.
- [ ] Apply `IsChannelQuotaExceeded()` in channel candidate selection before weight calculation.
- [ ] Apply persisted key quota checks in `GetNextEnabledKey()` before random/polling selection.
- [ ] Add `CanEnableChannel()` and `CanEnableChannelKey()` model helpers.
- [ ] Call the guards from existing channel status and multi-key enable actions.
- [ ] Run `go test ./model ./controller -run 'Test.*Quota.*Enable|Test.*Quota.*Selection' -count=1`.
- [ ] Commit with `git commit -m "fix: block scheduling and enable for exhausted quotas"`.

## Task 8: Add Reset, Key Limit, and Batch Stats APIs

- [ ] Add controller contract tests for batch stats, channel reset, key reset, key limit update, invalid mode, negative limit, and unknown fingerprint.
- [ ] Implement request DTOs with validation: mode enum, non-negative integer quota, and maximum batch ID count.
- [ ] Implement `GET /api/channel/usage/batch` with one batch query and zero-filled map response.
- [ ] Implement `GET /api/channel/:id/key-usages` returning current index, mask, fingerprint, used, limit, status, and reason.
- [ ] Implement channel and key reset endpoints that clear usage only.
- [ ] Implement key quota update endpoint without automatic status changes.
- [ ] Register routes under the existing admin-authenticated channel route in `router/api-router.go`.
- [ ] Run `go test ./controller -run 'TestChannelUsageAPI|TestChannelQuotaResetAPI' -count=1`.
- [ ] Commit with `git commit -m "feat: expose channel quota administration APIs"`.

## Task 9: Extend Channel Edit UI

- [ ] Add a frontend test that single-key channels only offer `none` and `channel`.
- [ ] Add a frontend test that multi-key channels offer all four modes.
- [ ] Add a test that `0` renders as unlimited and explanatory copy states reset does not enable.
- [ ] Add quota fields to the edit form initial state and submit payload in `EditChannelModal.jsx`.
- [ ] Add mode-dependent validation and hide irrelevant inputs without deleting their server values until submit normalization.
- [ ] Add i18n strings for modes, unlimited, reset behavior, and validation.
- [ ] Run the targeted frontend test command used by the repository for channel components.
- [ ] Commit with `git commit -m "feat: add channel quota settings UI"`.

## Task 10: Add Multi-Key Quota Management UI

- [ ] Add tests for loading key usages, masked-key display, editing a key limit, resetting usage, and exhausted enable errors.
- [ ] Extend `MultiKeyManageModal.jsx` with used/limit text, progress bar, status reason, edit action, and reset action.
- [ ] Use the server-provided fingerprint for quota operations and the current index only for existing compatibility actions.
- [ ] Require explicit confirmation before reset and state that reset will not enable the key.
- [ ] Refresh only the modal data after edit/reset; refresh the parent channel list after status-affecting operations.
- [ ] Run targeted channel modal tests.
- [ ] Commit with `git commit -m "feat: manage per-key channel quotas"`.

## Task 11: Replace Channel Usage Columns

- [ ] Add tests for `$today / $30d`, `$used / ∞`, finite limits, unavailable balance, loading, and error rendering.
- [ ] Implement `useChannelUsageStats.js` with one request per visible page and a monotonically increasing request sequence.
- [ ] Skip the request when all three usage columns are hidden.
- [ ] Add `ChannelUsageCells.jsx` and keep each cell to two compact text lines at most.
- [ ] Replace the existing usage/balance column definitions with `今日 / 30 日`, `已用 / 限额`, and `上游余额`.
- [ ] Preserve compact/adaptive table behavior and existing column preference storage.
- [ ] Run targeted frontend tests and `npm run build` from `web`.
- [ ] Commit with `git commit -m "feat: show channel usage and limits in channel table"`.

## Task 12: Add User Ranking Metric Switch

- [ ] Add helper tests where the same users rank differently by quota, tokens, and requests.
- [ ] Extend `processUserData()` to return all three aggregate values without changing the existing trend output.
- [ ] Add `userRankMetric` state with values `quota`, `tokens`, and `count`; default to `quota`.
- [ ] Build the active ranking data in `useDashboardCharts.jsx` with metric-specific sorting, axis formatting, tooltip formatting, and total text.
- [ ] Add a compact Semi UI segmented control inside the user ranking tab in `ChartsPanel.jsx`.
- [ ] Rename the tab from `用户消耗排行` to `用户排行` and add i18n strings for the three metrics.
- [ ] Keep model request ranking and user quota trend unchanged.
- [ ] Run dashboard helper/component tests and `npm run build` from `web`.
- [ ] Commit with `git commit -m "feat: add token and request user rankings"`.

## Task 13: Add Optional 30-Day Backfill Command

- [ ] Add tests for batched channel-level aggregation from raw logs and for resume checkpoints.
- [ ] Implement a command or root-only maintenance endpoint that aggregates recent logs into channel summary daily rows.
- [ ] Do not backfill key-level rows.
- [ ] Process bounded date/channel batches and persist a checkpoint so interruption is safe.
- [ ] Keep this operation opt-in; do not execute it during startup or migration.
- [ ] Document the command, expected lock impact, and rollback procedure.
- [ ] Commit with `git commit -m "feat: add channel usage history backfill"`.

## Task 14: Cross-Database and Regression Verification

- [ ] Run model and service quota tests against SQLite.
- [ ] Run the same focused integration suite against MySQL.
- [ ] Run PostgreSQL tests if the local test environment is available; otherwise inspect generated SQL and record the validation gap.
- [ ] Run `go test ./... -count=1`.
- [ ] Run the repository's frontend lint/test command and `npm run build` in `web`.
- [ ] Run `git diff --check`.
- [ ] Verify no response includes plaintext channel keys.
- [ ] Verify all old direct channel usage increments were replaced.
- [ ] Verify reset endpoints never change status and enable endpoints reject exhausted objects.
- [ ] Commit final test/documentation adjustments with `git commit -m "test: verify channel quota and ranking features"`.

## Release Order

1. Deploy schema and backend settlement code with UI hidden behind a feature option.
2. Observe channel usage and daily aggregation without finite limits for at least one settlement cycle.
3. Enable the channel table columns and reset APIs.
4. Configure one low-risk test channel with a finite channel limit and verify automatic disable.
5. Test one multi-key channel with a per-key limit and verify sibling failover.
6. Enable the user ranking metric switch.
7. Optionally run the 30-day channel-level backfill during a low-traffic window.

## Rollback

- Set every channel `quota_limit_mode` to `none` before rolling back application code.
- Keep new tables and columns; they are additive and do not affect older binaries.
- Restore old frontend columns if necessary; `used_quota` and `balance` remain intact.
- Do not drop quota tables until a later cleanup release confirms no rollback is required.
