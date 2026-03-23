package service

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"claude2api/model"
	"claude2api/utils"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AnthropicMessagesHandler handles the /v1/messages endpoint (Anthropic Messages API)
func AnthropicMessagesHandler(c *gin.Context) {
	var req model.AnthropicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "messages: Field required",
			},
		})
		return
	}

	// Convert Anthropic messages to internal format
	internalMessages := convertAnthropicMessages(req.System, req.Messages)

	// Process messages into prompt and extract images
	processor := utils.NewChatRequestProcessor()
	processor.ProcessMessages(internalMessages)

	// Get model or use default
	requestModel := getModelOrDefault(req.Model)
	index := config.Sr.NextIndex()

	// Attempt with retry mechanism
	for i := 0; i < config.ConfigInstance.RetryCount; i++ {
		index = (index + 1) % len(config.ConfigInstance.Sessions)
		session, err := config.ConfigInstance.GetSessionForModel(index)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get session for model %s: %v", requestModel, err))
			logger.Info("Retrying another session")
			continue
		}

		logger.Info(fmt.Sprintf("[Anthropic] Using session for model %s: %s", requestModel, session.SessionKey))
		if i > 0 {
			processor.Prompt.Reset()
			processor.Prompt.WriteString(processor.RootPrompt.String())
		}

		if handleAnthropicRequest(c, session, requestModel, processor, req.Stream) {
			return
		}

		logger.Info("Retrying another session")
	}

	logger.Error("Failed for all retries")
	c.JSON(http.StatusInternalServerError, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "api_error",
			"message": "Failed to process request after multiple attempts",
		},
	})
}

// handleAnthropicRequest processes a single request attempt with Anthropic response format
func handleAnthropicRequest(c *gin.Context, session config.SessionInfo, requestModel string, processor *utils.ChatRequestProcessor, stream bool) bool {
	claudeClient := core.NewClient(session.SessionKey, config.ConfigInstance.Proxy, requestModel)

	// Get org ID if not already set
	// If GetOrgID fails (e.g., 403 error), return false to trigger retry with another session
	// instead of continuing with empty org ID which would cause CreateConversation to fail
	if session.OrgID == "" {
		orgId, err := claudeClient.GetOrgID()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get org ID: %v", err))
			return false
		}
		session.OrgID = orgId
		config.ConfigInstance.SetSessionOrgID(session.SessionKey, session.OrgID)
	}

	claudeClient.SetOrgID(session.OrgID)

	// Upload images if any
	if len(processor.ImgDataList) > 0 {
		if err := claudeClient.UploadFile(processor.ImgDataList); err != nil {
			logger.Error(fmt.Sprintf("Failed to upload file: %v", err))
			return false
		}
	}

	// Create conversation
	if processor.Prompt.Len() > config.ConfigInstance.MaxChatHistoryLength {
		claudeClient.SetBigContext(processor.Prompt.String())
		processor.ResetForBigContext()
	}

	conversationID, err := claudeClient.CreateConversation()
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to create conversation: %v", err))
		return false
	}

	// For Anthropic format, we need to use SendMessageAnthropic which returns Anthropic Messages API format
	status, err := claudeClient.SendMessageAnthropic(conversationID, processor.Prompt.String(), stream, requestModel, c)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to send message: %v", err))
		go cleanupConversation(claudeClient, conversationID, 3)
		return false
	}

	if status != 200 {
		logger.Error(fmt.Sprintf("Unexpected status code: %d", status))
		go cleanupConversation(claudeClient, conversationID, 3)
		return false
	}

	// Cleanup conversation if needed
	if config.ConfigInstance.ChatDelete {
		go cleanupConversation(claudeClient, conversationID, 3)
	}

	return true
}

// convertAnthropicMessages converts Anthropic Messages API format to the internal format
func convertAnthropicMessages(system interface{}, messages []map[string]interface{}) []map[string]interface{} {
	var result []map[string]interface{}

	// Handle system message
	if system != nil {
		switch v := system.(type) {
		case string:
			if v != "" {
				result = append(result, map[string]interface{}{
					"role":    "system",
					"content": v,
				})
			}
		case []interface{}:
			var systemText string
			for _, block := range v {
				if blockMap, ok := block.(map[string]interface{}); ok {
					if blockMap["type"] == "text" {
						if text, ok := blockMap["text"].(string); ok {
							systemText += text
						}
					}
				}
			}
			if systemText != "" {
				result = append(result, map[string]interface{}{
					"role":    "system",
					"content": systemText,
				})
			}
		}
	}

	// Convert each message
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content := msg["content"]

		converted := map[string]interface{}{
			"role": role,
		}

		switch v := content.(type) {
		case string:
			converted["content"] = v

		case []interface{}:
			var contentParts []interface{}
			for _, block := range v {
				blockMap, ok := block.(map[string]interface{})
				if !ok {
					continue
				}
				blockType, _ := blockMap["type"].(string)
				switch blockType {
				case "text":
					contentParts = append(contentParts, map[string]interface{}{
						"type": "text",
						"text": blockMap["text"],
					})

				case "image":
					if source, ok := blockMap["source"].(map[string]interface{}); ok {
						mediaType, _ := source["media_type"].(string)
						data, _ := source["data"].(string)
						if mediaType != "" && data != "" {
							dataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, data)
							contentParts = append(contentParts, map[string]interface{}{
								"type": "image_url",
								"image_url": map[string]interface{}{
									"url": dataURL,
								},
							})
						}
					}
				}
			}
			if len(contentParts) > 0 {
				converted["content"] = contentParts
			}
		}

		result = append(result, converted)
	}

	return result
}
