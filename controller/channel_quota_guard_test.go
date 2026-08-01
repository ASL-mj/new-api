package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelQuotaGuardControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	initModelListColumnNames(t)
	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousCryptoSecret := common.CryptoSecret

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = false
	common.CryptoSecret = "controller-channel-quota-guard-secret"
	t.Setenv("CRYPTO_SECRET", common.CryptoSecret)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.Option{},
		&model.ChannelKeyUsage{},
		&model.ChannelUsageDaily{},
	))

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.CryptoSecret = previousCryptoSecret
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func decodeChannelQuotaGuardResponse(t *testing.T, recorderBody string) map[string]interface{} {
	t.Helper()
	var response map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(recorderBody), &response))
	return response
}

func TestUpdateChannelRejectsEnableUntilChannelQuotaIsReset(t *testing.T) {
	db := setupChannelQuotaGuardControllerTestDB(t)

	channel := &model.Channel{
		Id:             401,
		Name:           "controller-enable-guard",
		Key:            "sk-controller",
		Status:         common.ChannelStatusAutoDisabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeChannel,
		QuotaLimit:     100,
		QuotaLimitUsed: 100,
		Group:          "default",
		Models:         "gpt-4o-mini",
	}
	require.NoError(t, db.Create(channel).Error)

	body := `{"id":401,"type":1,"name":"controller-enable-guard","status":1,"models":"gpt-4o-mini","group":"default","quota_limit_mode":"channel","quota_limit":100,"quota_limit_used":100}`
	recorder := performMonitorGroupRequest(t, http.MethodPut, "/api/channel/", body, UpdateChannel)
	response := decodeChannelQuotaGuardResponse(t, recorder.Body.String())
	assert.Equal(t, false, response["success"])
	assert.Contains(t, response["message"], "重置")

	var reloaded model.Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, reloaded.Status)
}

func TestManageMultiKeysRejectsExhaustedKeyThenEnablesAfterReset(t *testing.T) {
	db := setupChannelQuotaGuardControllerTestDB(t)

	channel := &model.Channel{
		Id:             402,
		Name:           "controller-key-enable-guard",
		Key:            "sk-key-guard",
		Status:         common.ChannelStatusAutoDisabled,
		QuotaLimitMode: model.ChannelQuotaLimitModeKey,
		Group:          "default",
		Models:         "gpt-4o-mini",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 1,
			MultiKeyStatusList: map[int]int{
				0: common.ChannelStatusAutoDisabled,
			},
			MultiKeyDisabledReason: map[int]string{
				0: model.ChannelKeyQuotaDisabledReason,
			},
		},
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": "All keys are disabled",
		"status_time":   common.GetTimestamp(),
	})
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	usages, err := model.EnsureChannelKeyUsageRecords(channel)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.ChannelKeyUsage{}).
		Where("id = ?", usages[0].Id).
		Updates(map[string]interface{}{
			"quota_limit":      50,
			"quota_limit_used": 50,
			"status":           common.ChannelStatusAutoDisabled,
			"disabled_reason":  model.ChannelKeyQuotaDisabledReason,
		}).Error)

	body := `{"channel_id":402,"action":"enable_key","key_index":0}`
	rejected := performMonitorGroupRequest(t, http.MethodPost, "/api/channel/multi_key/manage", body, ManageMultiKeys)
	rejectedResponse := decodeChannelQuotaGuardResponse(t, rejected.Body.String())
	assert.Equal(t, false, rejectedResponse["success"])
	assert.Contains(t, rejectedResponse["message"], "重置")

	require.NoError(t, db.Model(&model.ChannelKeyUsage{}).
		Where("id = ?", usages[0].Id).
		Update("quota_limit_used", 0).Error)

	enabled := performMonitorGroupRequest(t, http.MethodPost, "/api/channel/multi_key/manage", body, ManageMultiKeys)
	enabledResponse := decodeChannelQuotaGuardResponse(t, enabled.Body.String())
	assert.Equal(t, true, enabledResponse["success"])

	var keyUsage model.ChannelKeyUsage
	require.NoError(t, db.First(&keyUsage, usages[0].Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, keyUsage.Status)
	assert.Empty(t, keyUsage.DisabledReason)
	assert.Zero(t, keyUsage.DisabledAt)

	var reloaded model.Channel
	require.NoError(t, db.First(&reloaded, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, reloaded.Status)

	var ability model.Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.True(t, ability.Enabled)
}
