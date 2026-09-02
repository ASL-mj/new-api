# Playground, Inline Menu, and Model Fetch Updates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 NewAPI v0.13.2 Fork 中完成操练场分组模型筛选、模型值映射、推理强度、外部内联菜单图标、用户总额度排序、README 补全和 Base URL 模型端点拼接修复。

**Architecture:** 保持 Router -> Controller -> Service -> Model 的后端分层，以及现有 React hooks/helpers/components 的前端组织方式。模型获取使用一个无副作用的 URL 拼接 helper；操练场通过可选 group 参数获取模型；外部菜单沿用现有 custom 配置和 `/external` 页面；不修改数据库用户输入的 Base URL，也不改变普通 Relay 转发链路。

**Tech Stack:** Go 1.22+, Gin, GORM, React 18, Vite, Semi UI, Lucide React, i18next, Bun。

---

### Task 1: 建立后端兼容 helper 和回归测试

**Files:**
- Create: `common/url.go`
- Test: `common/url_test.go`
- Modify: `controller/channel.go`
- Modify: `controller/channel_upstream_update.go`
- Modify: `controller/ratio_sync.go`
- Modify: `relay/channel/gemini/relay-gemini.go`
- Modify: `relay/channel/ollama/relay-ollama.go`

- [ ] **Step 1: Write failing URL joining tests**

Add table-driven tests for `common.JoinBaseURLPath`:

```go
func TestJoinBaseURLPath(t *testing.T) {

    tests := []struct {
        name, baseURL, endpoint, want string
    }{
        {"no slash", "https://example.com", "/v1/models", "https://example.com/v1/models"},
        {"one slash", "https://example.com/", "/v1/models", "https://example.com/v1/models"},
        {"many slash", "https://example.com///", "v1/models", "https://example.com/v1/models"},
        {"path", "https://example.com/api/", "/v1/models", "https://example.com/api/v1/models"},
        {"protocol untouched", "https://example.com", "v1/models", "https://example.com/v1/models"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            require.Equal(t, tt.want, JoinBaseURLPath(tt.baseURL, tt.endpoint))
        })
    }
}
```

Run `go test ./common -run TestJoinBaseURLPath -v`; it must fail because the helper does not exist yet.

- [ ] **Step 2: Implement the helper without URL corruption**

Implement `JoinBaseURLPath` in `common/url.go`:

```go
func JoinBaseURLPath(baseURL, endpoint string) string {
    baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
    endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
    return baseURL + endpoint
}
```

Do not use `path.Join`, and do not mutate a channel model or persisted Base URL.

- [ ] **Step 3: Run the helper tests**

Run `go test ./common -run TestJoinBaseURLPath -v`; expect PASS.

- [ ] **Step 4: Replace model-fetch endpoint formatting**

Replace direct `fmt.Sprintf("%s/...", baseURL)` only in model-list acquisition paths with `common.JoinBaseURLPath`. Keep existing empty Base URL fallback before joining. Use it for generic `/v1/models`, Ali `/compatible-mode/v1/models`, special provider endpoints, Gemini `/v1beta/models`, Ollama `/api/tags`, and ratio-sync model URLs.

For `constant.ChannelSpecialBases`, look up both the original trimmed URL and `strings.TrimRight(baseURL, "/")`, preferring the original key first. This preserves existing exact mappings while making a trailing slash compatible.

- [ ] **Step 5: Add endpoint-level regression coverage**

Use `httptest.Server` in controller/relay package tests to assert requests with Base URLs ending in `/` arrive at `/v1/models`, `/v1beta/models`, `/api/tags`, or the appropriate special path, never at a path containing `//` after the host.

- [ ] **Step 6: Run backend fetch tests**

Run `go test ./controller ./relay/channel/gemini ./relay/channel/ollama ./common`; expect PASS.

- [ ] **Step 7: Commit the isolated URL fix**

```bash
git add common/url.go common/url_test.go controller/channel.go controller/channel_upstream_update.go controller/ratio_sync.go relay/channel/gemini/relay-gemini.go relay/channel/ollama/relay-ollama.go
git commit -m "fix: normalize base urls when fetching models"
```

### Task 2: Add group-aware playground model loading

**Files:**
- Modify: `controller/user.go`
- Modify: `web/src/constants/playground.constants.js`
- Modify: `web/src/hooks/playground/useDataLoader.js`
- Modify: `web/src/hooks/playground/usePlaygroundState.js`
- Modify: `web/src/pages/Playground/index.jsx`
- Modify: `web/src/helpers/api.js`
- Test: `controller/user_models_test.go`
- Test: `web/src/helpers/__tests__/playground.test.js`

- [ ] **Step 1: Add backend tests for optional group filtering**

Cover `/api/user/models` with no group and with `group=selected-group`; assert the no-group response remains a string array and the group response contains only enabled models from the selected usable group. The test must also cover an invalid group returning no unauthorized model.

- [ ] **Step 2: Implement optional group query handling**

In `GetUserModels`, read `c.Query("group")`. When empty, retain the existing union of `service.GetUserUsableGroups(user.Group)`. When non-empty, authorize it against the user's usable groups and return only that group's enabled models. Preserve the response envelope and string-array data shape.

- [ ] **Step 3: Load models after group resolution**

Update `useDataLoader` so `loadModels` calls:

```js
API.get(API_ENDPOINTS.USER_MODELS, {
  params: inputs.group ? { group: inputs.group } : undefined,
});
```

Make group changes trigger a model reload. After each reload, retain the current model only when its option value still exists; otherwise clear it and show the existing translated re-selection notice. Keep `auto` behavior compatible with the current group options.

- [ ] **Step 4: Normalize model options**

Update `processModelsData` to accept strings and `{label, value}` objects, returning `{label, value}` options while selecting by `value`. Update `SettingsPanel` to pass and display those options without changing the payload value.

- [ ] **Step 5: Run playground tests and frontend lint for changed files**

Run `bunx vitest run web/src/helpers/__tests__/playground.test.js` if the repository test runner is available; otherwise run the repository's existing frontend test command and `bun run eslint -- web/src/helpers/api.js web/src/hooks/playground web/src/pages/Playground/index.jsx web/src/components/playground/SettingsPanel.jsx`. Expect no unauthorized models after group changes.

### Task 3: Add reasoning-effort configuration and preserve mapping values

**Files:**
- Modify: `web/src/constants/playground.constants.js`
- Modify: `web/src/components/playground/configStorage.js`
- Modify: `web/src/components/playground/SettingsPanel.jsx`
- Modify: `web/src/components/playground/ParameterControl.jsx`
- Modify: `web/src/helpers/api.js`
- Modify: `web/src/pages/Playground/index.jsx`
- Modify: `web/src/i18n/customKeys.js`
- Modify: `web/src/i18n/locales/zh-CN.json`
- Modify: `web/src/i18n/locales/zh-TW.json`
- Modify: `web/src/i18n/locales/en.json`
- Modify: `web/src/i18n/locales/fr.json`
- Modify: `web/src/i18n/locales/ru.json`
- Modify: `web/src/i18n/locales/ja.json`
- Modify: `web/src/i18n/locales/vi.json`
- Test: `web/src/helpers/__tests__/playground.test.js`

- [ ] **Step 1: Add failing payload tests**

Cover explicit and default reasoning effort:

```js
expect(buildApiPayload([], '', { model: 'shown-model', group: 'default', stream: true, reasoning_effort: 'xhigh' }, {})).toMatchObject({ reasoning_effort: 'xhigh' });
expect(buildApiPayload([], '', { model: 'shown-model', group: 'default', stream: true, reasoning_effort: '' }, {})).not.toHaveProperty('reasoning_effort');
```

Also assert a model option `{label: 'Mapped name', value: 'origin-id'}` keeps `origin-id` as the payload model.

- [ ] **Step 2: Add default configuration and UI selection**

Add `reasoning_effort: ''` to `DEFAULT_CONFIG.inputs`. Add a translated Select in the existing settings panel with `系统默认`, `none`, `low`, `medium`, `high`, and `xhigh`. Preserve custom request mode behavior.

- [ ] **Step 3: Add the payload field with omission semantics**

In `buildApiPayload`, add `reasoning_effort` only when it is a non-empty string. Do not replace `inputs.model` with an option label; the value remains the model sent to the playground API.

- [ ] **Step 4: Verify storage and replay paths**

Ensure `configStorage.js` merges the new input through `DEFAULT_CONFIG`, and that import/export, preview, send, and retry use the same input object. Add tests for loading an old config with no field and round-tripping an explicit value.

- [ ] **Step 5: Verify backend traceability compatibility**

Run existing Go tests for request parsing and log generation, confirming the existing `RelayInfo.ReasoningEffort` and `Other.reasoning_effort` paths remain used. Do not add a database column.

### Task 4: Add Lucide icons and inline external menu integration

**Files:**
- Modify: `web/src/helpers/externalMenu.js`
- Modify: `web/src/helpers/render.jsx`
- Modify: `web/src/hooks/common/useSidebar.js`
- Modify: `web/src/components/layout/SiderBar.jsx`
- Modify: `web/src/components/layout/headerbar/Navigation.jsx`
- Modify: `web/src/pages/External/index.jsx`
- Modify: `web/src/pages/Setting/Operation/SettingsSidebarModulesAdmin.jsx`
- Modify: `web/src/components/settings/personal/cards/NotificationSettings.jsx`
- Modify: `web/src/i18n/customKeys.js`
- Modify: `web/src/i18n/locales/*.json`
- Test: `web/src/helpers/__tests__/externalMenu.test.js`

- [ ] **Step 1: Define a bounded icon registry and fallback**

Create a registry mapping persisted names to imported Lucide components. `getExternalMenuIcon(iconName, selected)` returns the named icon when valid and `ExternalLink` otherwise. Extend normalization with `icon`, defaulting to `ExternalLink` without breaking old records.

- [ ] **Step 2: Extend administrator configuration**

Add an icon Select with preview to the existing custom-menu form. Persist only the icon name, retain `description`, `placement`, `enabled`, and `openMode`, and keep the five existing placement values without introducing a sixth sidebar area.

- [ ] **Step 3: Render icons in every placement**

Pass the normalized `iconKey` through `getCustomExternalMenuItems`; render it in SiderBar and Navigation. Keep the existing `/external/:id` and `/console/external/:id` route mapping.

- [ ] **Step 4: Preserve right-side inline behavior**

Keep the existing External page embedded container for sidebar routes. Confirm topbar and admin placement use their existing route behavior; do not turn sidebar menus into full-page replacement. Preserve user-controlled visibility for chat, console, and personal placements.

- [ ] **Step 5: Add compatibility tests**

Test old config without icon, valid icon, invalid icon, each placement, admin enabled=false, and user custom=false. Verify no fifth sidebar option appears and descriptions are shown to users instead of raw URLs.

### Task 5: Change user sorting to total quota

**Files:**
- Modify: `model/user.go`
- Modify: `web/src/hooks/users/useUsersData.jsx`
- Modify: `web/src/components/table/users/UsersColumnDefs.jsx`
- Modify: `web/src/i18n/customKeys.js`
- Modify: `web/src/i18n/locales/*.json`
- Test: `model/user_test.go`

- [ ] **Step 1: Add sorting tests**

Test `userSortClause("total_quota", "asc")` returns an expression equivalent to `quota + used_quota ASC, id ASC`, descending reverses both directions, and unknown fields return the default clause. Keep `aff_count` coverage.

- [ ] **Step 2: Update backend sort mapping**

Replace the `quota` sort key with `total_quota`; build the total expression through GORM-safe SQL expression text supported by SQLite, MySQL, and PostgreSQL. Keep `id` as the stable tie-breaker.

- [ ] **Step 3: Update frontend header and query key**

Change only the quota header's sort key to `total_quota`; remove the remaining-quota sort key and keep the existing three-state toggle behavior and invite-count sorting.

- [ ] **Step 4: Run user sorting tests**

Run `go test ./model -run TestUserSortClause -v` and the frontend ESLint check for user table files.

### Task 6: Complete README without removing protected attribution

**Files:**
- Modify: `README.md`
- Modify: `README.zh_CN.md`

- [ ] **Step 1: Add Fork feature overview**

Add a clearly separated Fork section covering all completed Fork modules: model performance, channel/group/operations monitoring, usage-log UI and token/reasoning display, channel quotas and multi-key management, custom inline menus, upstream settlement/client disconnect fixes, internationalization, and this round's playground and model-fetch changes.

- [ ] **Step 2: Document configuration and startup**

Document local prerequisites, Bun frontend commands, Go backend commands, Docker-only database setup, environment variables, model fetching Base URL behavior, menu placement/icon fields, reasoning effort, and user sort semantics.

- [ ] **Step 3: Add screenshots and limitations honestly**

Reference only existing repository screenshots/assets. State when a runtime screenshot was not captured. List limitations including upstream/provider differences, iframe CSP restrictions, and no automatic reset for quota limits.

- [ ] **Step 4: Verify README protection and formatting**

Run `rg -n "NewAPI|QuantumNous|AGPL" README.md README.zh_CN.md` and ensure protected identity, license, and upstream attribution remain intact.

### Task 7: Full local validation without starting services

**Files:**
- Modify: any files required by test/lint failures from Tasks 1-6 only

- [ ] **Step 1: Run Go formatting and targeted tests**

Run `gofmt -w` only on changed Go files, then `go test ./common ./controller ./model ./relay/channel/gemini ./relay/channel/ollama`.

- [ ] **Step 2: Run frontend checks**

From `web/`, run `bun run i18n:lint`, `bun run eslint`, `bun run lint`, and `bun run build`.

- [ ] **Step 3: Run repository diff checks**

Run `git diff --check` and inspect `git status --short`; keep `.superpowers/` and `new-api-deploy-20260901-115403` untouched and uncommitted.

- [ ] **Step 4: Review changed behavior against the seven acceptance groups**

Confirm model group filtering, label/value separation, reasoning omission/default behavior, icon fallback and inline placement, total quota sorting, README completeness, and one-slash model endpoint construction. Do not start Go, Vite, Docker, MySQL, Redis, or any production service.

- [ ] **Step 5: Commit implementation as reviewable commits**

Use focused commits from Tasks 1-6. Before each commit run the relevant targeted test; do not push or deploy in this task.
