package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/task_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func taskVideoControllerDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldRedis := model.DB, common.RedisEnabled
	common.RedisEnabled = false
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dashboard.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() { model.DB, common.RedisEnabled = oldDB, oldRedis; sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Task{}, &model.TaskSettlement{}, &model.Option{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTaskVideoResponseEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		upstream, want int
		success        bool
	}{
		{"public success", `{"id":"task_fixture123","object":"video","metadata":{"upstream_task_id":"PRIVATE_FIXTURE","key":"PRIVATE_FIXTURE"}}`, 201, 200, true},
		{"provider ID rejected", `{"id":"provider-private-id"}`, 200, 502, false},
		{"malformed success", `broken`, 200, 502, false},
		{"upstream denied", `{"error":{"message":"PRIVATE_FIXTURE"}}`, 403, 403, false},
		{"upstream error", `{"message":"PRIVATE_FIXTURE"}`, 500, 500, false},
		{"oversized response", strings.Repeat("x", taskVideoResponseLimit+1), 200, 502, false},
		{"redirect", `PRIVATE_FIXTURE`, 307, 502, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			controllerFinished := false
			r.POST("/api/task/video", TaskVideoResponse(), func(c *gin.Context) {
				c.Header("X-Provider-Key", "PRIVATE_FIXTURE")
				c.Header("Location", "https://example.invalid/PRIVATE_FIXTURE")
				c.Data(tc.upstream, "application/json", []byte(tc.body))
				// The real relay finishes its submission ledger before returning.
				controllerFinished = true
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/task/video", nil))
			if !controllerFinished || w.Code != tc.want {
				t.Fatalf("status=%d finished=%v", w.Code, controllerFinished)
			}
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					TaskID string `json:"task_id"`
				} `json:"data"`
			}
			if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Success != tc.success {
				t.Fatal(w.Body.String())
			}
			if tc.success && response.Data.TaskID != "task_fixture123" {
				t.Fatal("public ID missing")
			}
			if strings.Contains(w.Body.String(), "PRIVATE_FIXTURE") || w.Header().Get("X-Provider-Key") != "" || w.Header().Get("Location") != "" {
				t.Fatal("provider detail leaked")
			}
		})
	}
}

func TestTaskVideoResponseBufferBound(t *testing.T) {
	w := &taskVideoWriter{header: http.Header{}, status: 200}
	chunk := strings.Repeat("x", taskVideoResponseLimit/2)
	for i := 0; i < 6; i++ {
		if n, err := w.WriteString(chunk); err != nil || n != len(chunk) {
			t.Fatal("buffer interrupted relay")
		}
	}
	if !w.overflow || w.body.Len() > taskVideoResponseLimit {
		t.Fatal("response buffer exceeded limit")
	}
	w.WriteHeader(500)
	if w.Status() != 200 {
		t.Fatal("written response status changed")
	}
}

func TestTaskVideoRoleProjection(t *testing.T) {
	task := &model.Task{ID: 1, TaskID: "task_public", PollingVersion: 2, SubmissionSettled: true, Quota: 100, ChannelId: 9, Status: model.TaskStatusFailure, FailReason: "PRIVATE_FIXTURE", LastPollErrorClass: "provider_terminal", LastPollErrorMessage: "PRIVATE_FIXTURE", PollFailures: 2, LastPollAt: 10, NextPollAt: 20, Data: []byte(`{"api_key":"PRIVATE_FIXTURE","task_id":"PRIVATE_FIXTURE"}`), Properties: model.Properties{OriginModelName: "MiniMax-H3", Input: "PRIVATE_FIXTURE"}, PrivateData: model.TaskPrivateData{Key: "PRIVATE_FIXTURE", UpstreamTaskID: "PRIVATE_FIXTURE", ResultURL: "https://example.invalid/result.mp4", BillingContext: &model.TaskBillingContext{TaskPricingSnapshot: &types.TaskPricingSnapshot{Version: "fixture-v1"}}}}
	settlement := &model.TaskSettlement{ID: 4, TaskID: 1, Kind: model.TaskSettlementKindRefund, State: model.TaskSettlementStateApplied, PreConsumedQuota: 100, QuotaDelta: -100, AppliedAt: 30}
	for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			item := relay.TaskModel2Dto(task)
			projectTaskVideo(item, task, settlement, role)
			if item.ActualQuota == nil || *item.ActualQuota != 0 || item.QuotaDelta == nil || *item.QuotaDelta != -100 || item.SettlementStatus != "applied" || item.SettledAt != 30 {
				t.Fatal("refund accounting projection mismatch")
			}
			encoded, err := common.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE_FIXTURE") {
				t.Fatal("private fields leaked")
			}
			var fields map[string]any
			if err := common.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["data"] != nil {
				t.Fatal("v2 raw data leaked")
			}
			for _, key := range []string{"poll_failures", "last_poll_error_class", "last_poll_error_message", "last_poll_at", "next_poll_at"} {
				_, exists := fields[key]
				if exists != (role >= common.RoleAdminUser) {
					t.Fatalf("incorrect role access to %s", key)
				}
			}
			for _, key := range []string{"settlement_id", "pricing_version"} {
				_, exists := fields[key]
				if exists != (role >= common.RoleRootUser) {
					t.Fatalf("incorrect role access to %s", key)
				}
			}
			if len(task.Data) == 0 || task.FailReason != "PRIVATE_FIXTURE" {
				t.Fatal("projection mutated source")
			}
		})
	}
}

func TestTaskVideoUnappliedAmountsAndLegacy(t *testing.T) {
	for _, state := range []model.TaskSettlementState{model.TaskSettlementStatePending, model.TaskSettlementStateApplying, model.TaskSettlementStateFailed, model.TaskSettlementStateApplied} {
		task := &model.Task{PollingVersion: 2, Quota: 100, SubmissionSettled: true}
		item := relay.TaskModel2Dto(task)
		settlement := &model.TaskSettlement{State: state, Kind: model.TaskSettlementKindSettle, PreConsumedQuota: 100, ActualQuota: common.GetPointer(120), QuotaDelta: 20, AppliedAt: 30}
		projectTaskVideo(item, task, settlement, common.RoleCommonUser)
		if (item.ActualQuota != nil) != (state == model.TaskSettlementStateApplied) || (item.QuotaDelta != nil) != (state == model.TaskSettlementStateApplied) {
			t.Fatal("unapplied billing shown as final")
		}
		if item.PreConsumedQuota == nil || *item.PreConsumedQuota != 100 {
			t.Fatal("precharge missing")
		}
	}
	legacy := &model.Task{Status: model.TaskStatusSuccess, Quota: 80, Data: []byte(`{"suno":"legacy-preview"}`)}
	item := relay.TaskModel2Dto(legacy)
	projectTaskVideo(item, legacy, nil, common.RoleCommonUser)
	if string(item.Data) != string(legacy.Data) || item.SettlementStatus != "legacy" || item.ActualQuota != nil {
		t.Fatal("legacy preview or unknown billing changed")
	}
	pending := &model.Task{PollingVersion: 2, SubmissionSettled: false, Quota: 100}
	item = relay.TaskModel2Dto(pending)
	projectTaskVideo(item, pending, nil, common.RoleCommonUser)
	if item.PreConsumedQuota != nil || item.SettlementStatus != "pending" || item.SettledAt != 0 {
		t.Fatal("uncommitted precharge shown as posted")
	}
}

func TestTaskVideoSettlementBatchAndUserScope(t *testing.T) {
	db := taskVideoControllerDB(t)
	for _, task := range []model.Task{{ID: 1, UserId: 1, TaskID: "task_self", PollingVersion: 2}, {ID: 2, UserId: 2, TaskID: "task_other", PollingVersion: 2}} {
		if err := db.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{1, 2} {
		row := model.TaskSettlement{TaskID: id, Kind: model.TaskSettlementKindSettle, State: model.TaskSettlementStateApplied, PreConsumedQuota: 100, ActualQuota: common.GetPointer(110), QuotaDelta: 10}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	queries := 0
	if err := db.Callback().Query().Before("gorm:query").Register("task_video_count", func(tx *gorm.DB) {
		if tx.Statement.Table == "task_settlements" {
			queries++
		}
	}); err != nil {
		t.Fatal(err)
	}
	items, err := tasksToDto([]*model.Task{{ID: 1, PollingVersion: 2}, {ID: 2, PollingVersion: 2}}, false, common.RoleCommonUser)
	if err != nil || len(items) != 2 || queries != 1 {
		t.Fatalf("batch query count=%d error=%v", queries, err)
	}
	ctx, w := newAuthenticatedContext(t, http.MethodGet, "/api/task/self?p=1&size=20&user_id=2", nil, 1)
	ctx.Set("role", common.RoleCommonUser)
	GetUserTask(ctx)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []dto.TaskDto `json:"items"`
		} `json:"data"`
	}
	if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || len(response.Data.Items) != 1 || response.Data.Items[0].TaskID != "task_self" {
		t.Fatal("task list user scope failed")
	}
	if err := db.Migrator().DropTable(&model.TaskSettlement{}); err != nil {
		t.Fatal(err)
	}
	ctx, w = newAuthenticatedContext(t, http.MethodGet, "/api/task/self", nil, 1)
	GetUserTask(ctx)
	if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Success {
		t.Fatal("settlement query failure reported as success")
	}
}

func TestTaskVideoH3PricingOptionValidation(t *testing.T) {
	db := taskVideoControllerDB(t)
	oldRules, registered := config.GlobalConfig.ExportAllConfigs()[task_setting.H3PricingOption]
	if !registered {
		t.Fatal("H3 option not registered")
	}
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		_ = task_setting.UpdateH3PricingRules(oldRules)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
	})
	valid := `{"version":"fixture-v1","base_quota":1000,"duration_base_seconds":5,"resolution_ratios":{"768P":1,"2K":2},"image_reference_ratio_per_image":0.1,"video_reference_ratio_per_second":0.1,"audio_reference_ratio_per_audio":0.1}`
	if err := model.UpdateOption(task_setting.H3PricingOption, valid); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []any{`{}`, `{"version":"broken"}`, `null`, map[string]any{"version": "not-a-string-value"}} {
		ctx, w := newAuthenticatedContext(t, http.MethodPut, "/api/option/", OptionUpdateRequest{Key: task_setting.H3PricingOption, Value: invalid}, 1)
		UpdateOption(ctx)
		if decodeAPIResponse(t, w).Success {
			t.Fatal("invalid pricing accepted")
		}
		var persisted model.Option
		if err := db.First(&persisted, "key = ?", task_setting.H3PricingOption).Error; err != nil {
			t.Fatal(err)
		}
		if persisted.Value != valid || config.GlobalConfig.ExportAllConfigs()[task_setting.H3PricingOption] != valid {
			t.Fatal("invalid pricing persisted or activated")
		}
	}
	ctx, w := newAuthenticatedContext(t, http.MethodGet, "/api/option/", nil, 1)
	GetOptions(ctx)
	if !strings.Contains(w.Body.String(), task_setting.H3PricingOption) {
		t.Fatal("registered option absent from dashboard")
	}
	if err := model.UpdateOption(task_setting.H3PricingOption, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := task_setting.GetH3PricingRules(); err == nil {
		t.Fatal("clearing pricing did not disable rule availability")
	}
}
