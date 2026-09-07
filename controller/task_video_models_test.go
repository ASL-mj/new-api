package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func taskVideoModelsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousRedis := common.RedisEnabled
	previousMemory := common.MemoryCacheEnabled
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedis
		common.MemoryCacheEnabled = previousMemory
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.User{Id: 1, Username: "video-owner", Status: common.UserStatusEnabled, Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	mapping := `{"MiniMax-H3":"minimax_h3","MiniMax-H3-creative":"minimax_h3","MiniMax-H3-legacy":"MiniMax-Hailuo-2.3"}`
	if err := db.Create(&model.Channel{Id: 7, Type: constant.ChannelTypeMiniMax, Status: common.ChannelStatusEnabled, Group: "default", Models: "MiniMax-H3,minimax_h3,MiniMax-H3-creative,MiniMax-H3-legacy,legacy-video", ModelMapping: &mapping}).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"MiniMax-H3", "minimax_h3", "MiniMax-H3-creative", "MiniMax-H3-legacy", "legacy-video"} {
		if err := db.Create(&model.Ability{Group: "default", Model: name, ChannelId: 7, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestGetTaskVideoModelsUsesOwnedTokenAndEnabledAbilities(t *testing.T) {
	db := taskVideoModelsTestDB(t)
	if err := db.Create(&model.Token{Id: 11, UserId: 1, Key: "video-token", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 1)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/task/video/models?token_id=11", nil)
	GetTaskVideoModels(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	if err := common.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || len(response.Data) != 3 || response.Data[0] != "MiniMax-H3" || response.Data[1] != "MiniMax-H3-creative" || response.Data[2] != "minimax_h3" {
		t.Fatalf("unexpected model response: %+v", response)
	}
}

func TestGetTaskVideoModelsAppliesTokenModelLimits(t *testing.T) {
	db := taskVideoModelsTestDB(t)
	if err := db.Create(&model.Token{Id: 12, UserId: 1, Key: "limited-video-token", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: "minimax_h3", Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", 1)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/task/video/models?token_id=12", nil)
	GetTaskVideoModels(c)
	if w.Code != http.StatusOK || !containsTaskVideoModel(w.Body.Bytes(), "minimax_h3") || containsTaskVideoModel(w.Body.Bytes(), "MiniMax-H3\"") {
		t.Fatalf("token model limits not applied: status=%d body=%s", w.Code, w.Body.String())
	}
}

func containsTaskVideoModel(body []byte, value string) bool {
	var response struct {
		Data []string `json:"data"`
	}
	if common.Unmarshal(body, &response) != nil {
		return false
	}
	for _, modelName := range response.Data {
		if modelName == value {
			return true
		}
	}
	return false
}
