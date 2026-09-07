package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskPollLeaseAndStaleResult(t *testing.T) {
	truncateTables(t)
	task := &Task{TaskID: "lease", Status: TaskStatusSubmitted, PollingVersion: 2, SubmissionSettled: true}
	insertTask(t, task)
	stale := *task
	won, err := ClaimTaskPoll(task, 100, 200)
	require.NoError(t, err)
	require.True(t, won)
	won, err = ClaimTaskPoll(&stale, 100, 200)
	require.NoError(t, err)
	require.False(t, won)
	ready, err := GetReadyTaskPolls(150, 10)
	require.NoError(t, err)
	require.Empty(t, ready)
	ready, err = GetReadyTaskPolls(201, 10)
	require.NoError(t, err)
	require.Len(t, ready, 1)
	won, err = ClaimTaskPoll(ready[0], 201, 300)
	require.NoError(t, err)
	require.True(t, won)
	task.NextPollAt = 250
	won, err = SaveTaskPoll(task, TaskStatusSubmitted, 200)
	require.NoError(t, err)
	require.False(t, won)
}

func TestReadyTaskPollsExcludeTerminalUnboundAndFuture(t *testing.T) {
	truncateTables(t)
	for _, status := range []TaskStatus{TaskStatusNotStart, TaskStatusSuccess, TaskStatusFailure, TaskStatusSubmitted, TaskStatusQueued, TaskStatusInProgress} {
		insertTask(t, &Task{TaskID: string(status), Status: status, PollingVersion: 2, SubmissionSettled: true})
	}
	insertTask(t, &Task{TaskID: "future", Status: TaskStatusSubmitted, NextPollAt: 200})
	insertTask(t, &Task{TaskID: "submission-ledger-pending", Status: TaskStatusSubmitted, PollingVersion: 2})
	insertTask(t, &Task{TaskID: "legacy-submission", Status: TaskStatusSubmitted, SubmissionSettled: true})
	tasks, err := GetReadyTaskPolls(100, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 3)
}
