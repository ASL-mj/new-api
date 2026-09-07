package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetTaskVideoModels returns the current user's enabled video models for one
// owned token. Only public model names are returned.
func GetTaskVideoModels(c *gin.Context) {
	tokenID, err := strconv.Atoi(c.Query("token_id"))
	if err != nil || tokenID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "A valid token_id is required"})
		return
	}

	userID := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Selected token is not available"})
		} else {
			common.ApiError(c, err)
		}
		return
	}
	models, err := service.GetTaskVideoModelsForToken(userID, token)
	if err != nil {
		status := http.StatusInternalServerError
		message := "Failed to load video models"
		if errors.Is(err, service.ErrTaskVideoTokenUnavailable) {
			status, message = http.StatusUnauthorized, "Selected token is not active"
		} else if errors.Is(err, service.ErrTaskVideoTokenForbidden) {
			status, message = http.StatusForbidden, "Selected token is not available"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
		return
	}
	common.ApiSuccess(c, models)
}
