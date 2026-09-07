package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func taskVideoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB, oldRedis, oldMemory := model.DB, common.RedisEnabled, common.MemoryCacheEnabled
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "video.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.MemoryCacheEnabled = oldDB, oldRedis, oldMemory
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}); err != nil {
		t.Fatal(err)
	}
	for _, user := range []model.User{
		{Id: 1, Username: "videoowner", AffCode: "vidone", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", Quota: 10000},
		{Id: 2, Username: "otherowner", AffCode: "vidtwo", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", Quota: 10000},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	channel := model.Channel{Id: 1, Name: "local-fixture", Type: constant.ChannelTypeMiniMax, Status: common.ChannelStatusEnabled, Key: "channelFixtureSecret", Group: "default", Models: "MiniMax-H3"}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatal(err)
	}
	ability := model.Ability{Group: "default", Model: "MiniMax-H3", ChannelId: 1, Enabled: true, Priority: common.GetPointer(int64(0)), Weight: 1}
	if err := db.Create(&ability).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func taskVideoTestRouter(t *testing.T, sessionStatus int, handlers ...gin.HandlerFunc) (*gin.Engine, *http.Cookie) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	r.Use(sessions.Sessions("video_test", cookie.NewStore([]byte("local-session-fixture-secret-32bytes"))))
	r.GET("/login-fixture", func(c *gin.Context) {
		s := sessions.Default(c)
		for k, v := range map[string]any{"id": 1, "username": "videoowner", "role": common.RoleCommonUser, "status": sessionStatus, "group": "default"} {
			s.Set(k, v)
		}
		if err := s.Save(); err != nil {
			t.Fatal(err)
		}
		c.Status(http.StatusNoContent)
	})
	r.POST("/api/task/video", handlers...)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/login-fixture", nil))
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing session fixture")
	}
	return r, cookies[0]
}

func TestTaskVideoAuthAndRestrictions(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		mutate          func(*model.Token)
		header          string
		noSession       bool
		disabledSession bool
		disabledUser    bool
	}{
		{name: "owned", status: 204},
		{name: "no session even with bearer", status: 401, noSession: true},
		{name: "header user switch", status: 401, header: "2"},
		{name: "missing header", status: 401, header: "missing"},
		{name: "disabled session", status: 403, disabledSession: true},
		{name: "other owner", status: 403, mutate: func(v *model.Token) { v.UserId = 2 }},
		{name: "disabled token", status: 401, mutate: func(v *model.Token) { v.Status = common.TokenStatusDisabled }},
		{name: "expired token", status: 401, mutate: func(v *model.Token) { v.ExpiredTime = 1 }},
		{name: "exhausted quota", status: 401, mutate: func(v *model.Token) { v.UnlimitedQuota = false; v.RemainQuota = 0 }},
		{name: "IP denied", status: 403, mutate: func(v *model.Token) { v.AllowIps = common.GetPointer("198.51.100.0/24") }},
		{name: "IP allowed", status: 204, mutate: func(v *model.Token) { v.AllowIps = common.GetPointer("192.0.2.0/24") }},
		{name: "model denied", status: 403, mutate: func(v *model.Token) { v.ModelLimitsEnabled = true; v.ModelLimits = "other-model" }},
		{name: "model allowed", status: 204, mutate: func(v *model.Token) { v.ModelLimitsEnabled = true; v.ModelLimits = "MiniMax-H3" }},
		{name: "group denied", status: 403, mutate: func(v *model.Token) { v.Group = "restricted-fixture" }},
		{name: "stored channel selector", status: 403, mutate: func(v *model.Token) { v.Key = "fixturekey-1" }},
		{name: "disabled DB user", status: 403, disabledUser: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := taskVideoTestDB(t)
			token := model.Token{Id: 1, UserId: 1, Key: "ownedTokenFixtureSecret", Name: "fixture", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, RemainQuota: 100, Group: "default"}
			if tc.mutate != nil {
				tc.mutate(&token)
			}
			if err := db.Create(&token).Error; err != nil {
				t.Fatal(err)
			}
			if tc.disabledUser {
				if err := db.Model(&model.User{}).Where("id = ?", 1).Update("status", common.UserStatusDisabled).Error; err != nil {
					t.Fatal(err)
				}
			}
			sessionStatus := common.UserStatusEnabled
			if tc.disabledSession {
				sessionStatus = common.UserStatusDisabled
			}
			reached := false
			r, session := taskVideoTestRouter(t, sessionStatus, TaskVideoSessionAuth(), TaskVideoToken(), TokenAuth(), TaskVideoIdentity(), Distribute(), func(c *gin.Context) {
				reached = true
				if c.GetInt("id") != 1 || c.GetInt("token_id") != 1 {
					t.Fatal("billing identity changed")
				}
				if c.Request.URL.Path != "/v1/videos" || c.Request.URL.RawQuery != "" || c.Request.RequestURI != "/v1/videos" {
					t.Fatal("unexpected relay route")
				}
				if c.GetInt("relay_mode") == 0 {
					t.Fatal("video relay mode missing")
				}
				if c.GetString("using_group") != "default" {
					t.Fatal("token group not applied")
				}
				var body map[string]any
				if err := common.UnmarshalBodyReusable(c, &body); err != nil {
					t.Fatal(err)
				}
				if _, ok := body["token_id"]; ok {
					t.Fatal("dashboard token id forwarded")
				}
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/api/task/video?key=other-key&channel_id=999", strings.NewReader(`{"token_id":1,"model":"MiniMax-H3","prompt":"fixture","duration":5,"size":"768P","metadata":{"ratio":"16:9","aigc_watermark":false}}`))
			req.RemoteAddr = "192.0.2.1:1234"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("New-API-User", "1")
			if tc.header == "missing" {
				req.Header.Del("New-API-User")
			} else if tc.header != "" {
				req.Header.Set("New-API-User", tc.header)
			}
			// Every alternate token transport is hostile input, never the billing identity.
			for _, key := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Mj-Api-Secret"} {
				req.Header.Set(key, "Bearer otherTokenFixtureSecret-999")
			}
			req.Header.Set("Sec-WebSocket-Protocol", "realtime, openai-insecure-api-key.otherTokenFixtureSecret-999")
			if !tc.noSession {
				req.AddCookie(session)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
			if reached != (tc.status == 204) {
				t.Fatal("incorrect downstream execution")
			}
			if strings.Contains(w.Body.String(), "FixtureSecret") {
				t.Fatal("credential in response")
			}
		})
	}
}

func TestTaskVideoRejectsBodyOverrides(t *testing.T) {
	for _, tc := range []struct {
		body   string
		status int
	}{
		{`null`, http.StatusBadRequest}, {`{}`, http.StatusBadRequest}, {`{"token_id":1.5,"model":"MiniMax-H3","prompt":"x"}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"other","prompt":"x"}`, http.StatusForbidden},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x","duration":4.5}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x","channel_id":1}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x","user_id":2}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x","api_key":"secret"}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x","metadata":{"api_key":"secret"}}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x","metadata":{"channel_id":1}}`, http.StatusBadRequest},
		{`{"token_id":1,"model":"MiniMax-H3","prompt":"x"} {}`, http.StatusBadRequest},
	} {
		t.Run(strconv.Itoa(len(tc.body))+tc.body[:1], func(t *testing.T) {
			db := taskVideoTestDB(t)
			if err := db.Create(&model.Token{Id: 1, UserId: 1, Key: "ownedTokenFixtureSecret", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"}).Error; err != nil {
				t.Fatal(err)
			}
			r, session := taskVideoTestRouter(t, common.UserStatusEnabled, TaskVideoSessionAuth(), TaskVideoToken(), func(c *gin.Context) { t.Fatal("invalid body reached relay") })
			req := httptest.NewRequest(http.MethodPost, "/api/task/video", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("New-API-User", "1")
			req.AddCookie(session)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
		})
	}
}

func TestTaskVideoBodyBoundAndIdentityGuard(t *testing.T) {
	r, session := taskVideoTestRouter(t, common.UserStatusEnabled, TaskVideoSessionAuth(), TaskVideoToken(), func(c *gin.Context) { t.Fatal("oversized body reached relay") })
	req := httptest.NewRequest(http.MethodPost, "/api/task/video", strings.NewReader(strings.Repeat("x", taskVideoBodyLimit+1)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("New-API-User", "1")
	req.AddCookie(session)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 413 {
		t.Fatalf("status=%d", w.Code)
	}
	for _, changed := range []string{"id", "token_id", "specific_channel_id"} {
		t.Run(fmt.Sprint(changed), func(t *testing.T) {
			g := gin.New()
			g.POST("/", func(c *gin.Context) {
				c.Set(taskVideoUserID, 1)
				c.Set(taskVideoTokenID, 1)
				c.Set("id", 1)
				c.Set("token_id", 1)
				c.Set(changed, 2)
			}, TaskVideoIdentity(), func(c *gin.Context) { t.Fatal("identity switch reached relay") })
			w := httptest.NewRecorder()
			g.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", nil))
			if w.Code != 403 {
				t.Fatalf("status=%d", w.Code)
			}
		})
	}
}

func TestTaskVideoAcceptsModelExposedBySelectedToken(t *testing.T) {
	db := taskVideoTestDB(t)
	if err := db.Create(&model.Token{Id: 1, UserId: 1, Key: "ownedTokenFixtureSecret", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Ability{Group: "default", Model: "minimax_h3", ChannelId: 1, Enabled: true, Weight: 1}).Error; err != nil {
		t.Fatal(err)
	}
	r, session := taskVideoTestRouter(t, common.UserStatusEnabled, TaskVideoSessionAuth(), TaskVideoToken(), TokenAuth(), TaskVideoIdentity(), Distribute(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/task/video", strings.NewReader(`{"token_id":1,"model":"minimax_h3","prompt":"fixture"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("New-API-User", "1")
	req.AddCookie(session)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
