package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestMappedRequestModelCanSelectChannelFromCache(t *testing.T) {
	previousCacheEnabled := common.MemoryCacheEnabled
	previousModelIndex := group2model2channels
	previousChannels := channelsIDM
	defer func() {
		common.MemoryCacheEnabled = previousCacheEnabled
		group2model2channels = previousModelIndex
		channelsIDM = previousChannels
	}()

	common.MemoryCacheEnabled = true
	mapping := `{"hy3":"b/hy3"}`
	channel := &Channel{
		Id:           1,
		Status:       common.ChannelStatusEnabled,
		Models:       "hy3",
		ModelMapping: &mapping,
	}
	group2model2channels = map[string]map[string][]int{
		"free": {},
	}
	channelsIDM = map[int]*Channel{channel.Id: channel}
	addChannelModelIndex(group2model2channels, "free", "hy3", channel.Id)
	addChannelModelIndex(group2model2channels, "free", "b/hy3", channel.Id)

	selected, err := GetRandomSatisfiedChannel("free", "b/hy3", 0)
	if err != nil {
		t.Fatalf("GetRandomSatisfiedChannel() error = %v", err)
	}
	if selected == nil || selected.Id != channel.Id {
		t.Fatalf("GetRandomSatisfiedChannel() = %#v, want channel %d", selected, channel.Id)
	}
}
