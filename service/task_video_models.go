package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var (
	ErrTaskVideoTokenUnavailable = errors.New("task video token unavailable")
	ErrTaskVideoTokenForbidden   = errors.New("task video token forbidden")
)

// GetTaskVideoModelsForToken resolves the effective token group and returns
// current enabled MiniMax video models. The token key is never part of the
// result.
func GetTaskVideoModelsForToken(userID int, token *model.Token) ([]string, error) {
	if token == nil || userID <= 0 || token.UserId != userID {
		return nil, ErrTaskVideoTokenForbidden
	}
	if token.Status != common.TokenStatusEnabled ||
		(token.ExpiredTime > 0 && token.ExpiredTime <= time.Now().Unix()) ||
		(!token.UnlimitedQuota && token.RemainQuota <= 0) {
		return nil, ErrTaskVideoTokenUnavailable
	}

	user, err := model.GetUserCache(userID)
	if err != nil {
		return nil, err
	}
	group := user.Group
	if token.Group != "" {
		if !GroupInUserUsableGroups(user.Group, token.Group) {
			return nil, ErrTaskVideoTokenForbidden
		}
		group = token.Group
	}

	candidates, err := model.GetEnabledModelsByGroupAndChannelTypes(group, []int{constant.ChannelTypeMiniMax})
	if err != nil {
		return nil, err
	}
	models := compatibleTaskVideoModels(candidates)
	if !token.ModelLimitsEnabled {
		return models, nil
	}

	limits := token.GetModelLimitsMap()
	filtered := make([]string, 0, len(models))
	for _, modelName := range models {
		matchingName := ratio_setting.FormatMatchingModelName(modelName)
		if limits[modelName] || (matchingName != "" && limits[matchingName]) {
			filtered = append(filtered, modelName)
		}
	}
	return filtered, nil
}

func compatibleTaskVideoModels(candidates []model.TaskVideoModelChannel) []string {
	compatible := make(map[string]bool, len(candidates))
	ordered := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if !seen[candidate.Model] {
			seen[candidate.Model] = true
			compatible[candidate.Model] = true
			ordered = append(ordered, candidate.Model)
		}
		if !taskVideoModelResolvesToH3(candidate.Model, candidate.ModelMapping) {
			compatible[candidate.Model] = false
		}
	}
	models := make([]string, 0, len(candidates))
	for _, modelName := range ordered {
		if compatible[modelName] {
			models = append(models, modelName)
		}
	}
	return models
}

func taskVideoModelResolvesToH3(modelName, rawMapping string) bool {
	modelName = strings.TrimSpace(modelName)
	if strings.TrimSpace(rawMapping) == "" || rawMapping == "{}" {
		return isTaskVideoH3Model(modelName)
	}
	mapping := make(map[string]string)
	if json.Unmarshal([]byte(rawMapping), &mapping) != nil {
		return false
	}
	visited := map[string]bool{modelName: true}
	for {
		next := strings.TrimSpace(mapping[modelName])
		if next == "" {
			return isTaskVideoH3Model(modelName)
		}
		if visited[next] {
			return false
		}
		if isTaskVideoH3Model(next) {
			return true
		}
		visited[next] = true
		modelName = next
	}
}

func isTaskVideoH3Model(modelName string) bool {
	switch strings.TrimSpace(modelName) {
	case "MiniMax-H3", "minimax_h3":
		return true
	default:
		return false
	}
}

func TaskVideoModelAllowed(models []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return false
	}
	for _, modelName := range models {
		if modelName == requested {
			return true
		}
	}
	return false
}
