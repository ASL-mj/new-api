package main

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestStartMonitorGroupWorkersOnMasterAndSlave(t *testing.T) {
	previousMaster := common.IsMasterNode
	previousStart := startMonitorGroupRunner
	t.Cleanup(func() {
		common.IsMasterNode = previousMaster
		startMonitorGroupRunner = previousStart
	})

	started := 0
	startMonitorGroupRunner = func() {
		started++
	}
	for _, isMaster := range []bool{false, true} {
		common.IsMasterNode = isMaster
		startMonitorGroupWorkers()
	}

	assert.Equal(t, 2, started)
}
