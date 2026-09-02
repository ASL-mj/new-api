package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

// ModelOption separates the model shown to a user from the upstream model value
// that must be sent back through the relay request.
type ModelOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type modelMappingRow struct {
	Model        string
	ModelMapping *string
}

type modelMappingAbilityRow struct {
	Ability
	ModelMapping *string `gorm:"column:model_mapping"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

// GetGroupModelOptions exposes a channel mapping to the playground. A mapping
// source remains the user-facing label while its resolved target is the model
// value sent in the request.
func GetGroupModelOptions(groups []string) ([]ModelOption, error) {
	if len(groups) == 0 {
		return []ModelOption{}, nil
	}

	var rows []modelMappingRow
	err := DB.Table("abilities").
		Select("abilities.model, channels.model_mapping").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where(qualifiedAbilityGroupColumn()+" IN ? AND abilities.enabled = ?", groups, true).
		Order("abilities.model ASC, channels.priority DESC, channels.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	options := make([]ModelOption, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		modelName := strings.TrimSpace(row.Model)
		if modelName == "" {
			continue
		}
		requestModel := resolveModelMappingTarget(modelName, row.ModelMapping)
		if _, ok := seen[requestModel]; ok {
			continue
		}
		seen[requestModel] = struct{}{}
		options = append(options, ModelOption{
			Label: modelName,
			Value: requestModel,
		})
	}
	return options, nil
}

func parseModelMapping(rawMapping *string) map[string]string {
	if rawMapping == nil || strings.TrimSpace(*rawMapping) == "" {
		return nil
	}

	mapping := make(map[string]string)
	if err := json.Unmarshal([]byte(*rawMapping), &mapping); err != nil {
		return nil
	}
	return mapping
}

func resolveModelMappingTarget(modelName string, rawMapping *string) string {
	return resolveModelMappingTargetWithMap(modelName, parseModelMapping(rawMapping))
}

func resolveModelMappingTargetWithMap(modelName string, mapping map[string]string) string {
	if len(mapping) == 0 {
		return modelName
	}

	current := modelName
	visited := map[string]struct{}{current: {}}
	for {
		next := strings.TrimSpace(mapping[current])
		if next == "" {
			return current
		}
		if _, exists := visited[next]; exists {
			return modelName
		}
		visited[next] = struct{}{}
		current = next
	}
}

// GetChannelModelForRequest converts an upstream model value back to the
// configured channel model. Relay uses that configured name for quota and
// pricing, then ModelMappedHelper applies the channel mapping before upstream.
func GetChannelModelForRequest(channel *Channel, requestModel string) string {
	if channel == nil || requestModel == "" {
		return requestModel
	}

	mapping := parseModelMapping(channel.ModelMapping)
	for _, configuredModel := range strings.Split(channel.Models, ",") {
		configuredModel = strings.TrimSpace(configuredModel)
		if configuredModel == "" {
			continue
		}
		if resolveModelMappingTargetWithMap(configuredModel, mapping) == requestModel {
			return configuredModel
		}
	}
	return requestModel
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func getPriority(group string, model string, retry int) (int, error) {

	var priorities []int
	groupColumn := qualifiedAbilityGroupColumn()
	err := schedulableAbilityQuery().
		Select("DISTINCT(abilities.priority)").
		Where(groupColumn+" = ? and abilities.model = ? and abilities.enabled = ?", group, model, true).
		Order("abilities.priority DESC").              // 按优先级降序排序
		Pluck("abilities.priority", &priorities).Error // Pluck用于将查询的结果直接扫描到一个切片中

	if err != nil {
		// 处理错误
		return 0, err
	}

	if len(priorities) == 0 {
		// 如果没有查询到优先级，则返回错误
		return 0, errors.New("数据库一致性被破坏")
	}

	// 确定要使用的优先级
	var priorityToUse int
	if retry >= len(priorities) {
		// 如果重试次数大于优先级数，则使用最小的优先级
		priorityToUse = priorities[len(priorities)-1]
	} else {
		priorityToUse = priorities[retry]
	}
	return priorityToUse, nil
}

func qualifiedAbilityGroupColumn() string {
	groupColumn := commonGroupCol
	if groupColumn == "" {
		if common.UsingPostgreSQL {
			groupColumn = `"group"`
		} else {
			groupColumn = "`group`"
		}
	}
	return "abilities." + groupColumn
}

func schedulableAbilityQuery() *gorm.DB {
	return DB.Model(&Ability{}).
		Select("abilities.*").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("channels.status = ?", common.ChannelStatusEnabled).
		Where(
			"channels.quota_limit <= 0 OR channels.quota_limit_mode NOT IN ? OR channels.quota_limit_used < channels.quota_limit",
			channelUsageQuotaLimitModes(),
		)
}

func getChannelQuery(group string, model string, retry int) (*gorm.DB, error) {
	groupColumn := qualifiedAbilityGroupColumn()
	maxPrioritySubQuery := schedulableAbilityQuery().
		Select("MAX(abilities.priority)").
		Where(groupColumn+" = ? and abilities.model = ? and abilities.enabled = ?", group, model, true)
	channelQuery := schedulableAbilityQuery().
		Where(groupColumn+" = ? and abilities.model = ? and abilities.enabled = ? and abilities.priority = (?)", group, model, true, maxPrioritySubQuery)
	if retry != 0 {
		priority, err := getPriority(group, model, retry)
		if err != nil {
			return nil, err
		} else {
			channelQuery = schedulableAbilityQuery().
				Where(groupColumn+" = ? and abilities.model = ? and abilities.enabled = ? and abilities.priority = ?", group, model, true, priority)
		}
	}

	return channelQuery, nil
}

func GetChannel(group string, model string, retry int) (*Channel, error) {
	var abilities []Ability

	var err error = nil
	channelQuery, err := getChannelQuery(group, model, retry)
	if err != nil {
		return nil, err
	}
	if common.UsingSQLite || common.UsingPostgreSQL {
		err = channelQuery.Order("abilities.weight DESC").Find(&abilities).Error
	} else {
		err = channelQuery.Order("abilities.weight DESC").Find(&abilities).Error
	}
	if err != nil {
		return nil, err
	}
	if len(abilities) == 0 {
		abilities, err = getMappedChannelAbilities(group, model, retry)
		if err != nil {
			return nil, err
		}
	}
	channel := Channel{}
	if len(abilities) > 0 {
		// Randomly choose one
		weightSum := uint(0)
		for _, ability_ := range abilities {
			weightSum += ability_.Weight + 10
		}
		// Randomly choose one
		weight := common.GetRandomInt(int(weightSum))
		for _, ability_ := range abilities {
			weight -= int(ability_.Weight) + 10
			//log.Printf("weight: %d, ability weight: %d", weight, *ability_.Weight)
			if weight <= 0 {
				channel.Id = ability_.ChannelId
				break
			}
		}
	} else {
		return nil, nil
	}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

func getMappedChannelAbilities(group string, requestModel string, retry int) ([]Ability, error) {
	var rows []modelMappingAbilityRow
	err := schedulableAbilityQuery().
		Select("abilities.*, channels.model_mapping").
		Where(qualifiedAbilityGroupColumn()+" = ? and abilities.enabled = ?", group, true).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	matched := make([]Ability, 0, len(rows))
	priorities := make(map[int64]struct{})
	for _, row := range rows {
		if resolveModelMappingTarget(row.Model, row.ModelMapping) != requestModel {
			continue
		}
		matched = append(matched, row.Ability)
		priorities[lo.FromPtr(row.Priority)] = struct{}{}
	}
	if len(matched) == 0 {
		return nil, nil
	}

	orderedPriorities := make([]int64, 0, len(priorities))
	for priority := range priorities {
		orderedPriorities = append(orderedPriorities, priority)
	}
	sort.Slice(orderedPriorities, func(i, j int) bool {
		return orderedPriorities[i] > orderedPriorities[j]
	})
	if retry >= len(orderedPriorities) {
		retry = len(orderedPriorities) - 1
	}
	targetPriority := orderedPriorities[retry]

	result := make([]Ability, 0, len(matched))
	for _, ability := range matched {
		if lo.FromPtr(ability.Priority) == targetPriority {
			result = append(result, ability)
		}
	}
	return result, nil
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
