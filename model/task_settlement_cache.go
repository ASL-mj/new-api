package model

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
)

// RefreshTaskChannelCache restores routing after an atomic task settlement.
// Database errors are returned so the durable effects queue can retry them.
func RefreshTaskChannelCache(channelID int) error {
	if !common.MemoryCacheEnabled {
		return nil
	}
	channel, err := GetChannelById(channelID, true)
	if err != nil {
		return err
	}
	var abilities []Ability
	if err := DB.Where("channel_id = ? AND enabled = ?", channelID, true).Find(&abilities).Error; err != nil {
		return err
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channelsIDM == nil {
		channelsIDM = make(map[int]*Channel)
	}
	if group2model2channels == nil {
		group2model2channels = make(map[string]map[string][]int)
	}
	if old := channelsIDM[channelID]; old != nil && old.Key == channel.Key {
		channel.ChannelInfo.MultiKeyPollingIndex = old.ChannelInfo.MultiKeyPollingIndex
	}
	channel.Keys = channel.GetKeys()
	channelsIDM[channelID] = channel
	for _, models := range group2model2channels {
		for modelName, ids := range models {
			filtered := make([]int, 0, len(ids))
			for _, id := range ids {
				if id != channelID {
					filtered = append(filtered, id)
				}
			}
			models[modelName] = filtered
		}
	}
	if channel.Status != common.ChannelStatusEnabled || channel.IsChannelQuotaExceeded() {
		return nil
	}
	for _, ability := range abilities {
		if group2model2channels[ability.Group] == nil {
			group2model2channels[ability.Group] = make(map[string][]int)
		}
		ids := append(group2model2channels[ability.Group][ability.Model], channelID)
		sort.SliceStable(ids, func(i, j int) bool {
			left, right := channelsIDM[ids[i]], channelsIDM[ids[j]]
			return left != nil && (right == nil || left.GetPriority() > right.GetPriority())
		})
		group2model2channels[ability.Group][ability.Model] = ids
	}
	return nil
}
