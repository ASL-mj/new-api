package model

import "strings"

type TaskVideoModelChannel struct {
	Model        string
	ChannelID    int
	ModelMapping string
}

// GetEnabledModelsByGroupAndChannelTypes returns the models that can be
// scheduled for a group from enabled channels of the requested types.
// It intentionally reads abilities rather than channel.Models so disabled
// per-channel model entries are excluded.
func GetEnabledModelsByGroupAndChannelTypes(group string, channelTypes []int) ([]TaskVideoModelChannel, error) {
	group = strings.TrimSpace(group)
	if group == "" || len(channelTypes) == 0 {
		return []TaskVideoModelChannel{}, nil
	}

	models := make([]TaskVideoModelChannel, 0)
	err := DB.Table("abilities").
		Select("abilities.model, channels.id AS channel_id, COALESCE(channels.model_mapping, '') AS model_mapping").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where(qualifiedAbilityGroupColumn()+" = ?", group).
		Where("abilities.enabled = ?", true).
		Where("channels.status = ?", 1).
		Where("channels.type IN ?", channelTypes).
		Order("abilities.model ASC, channels.id ASC").
		Scan(&models).Error
	return models, err
}
