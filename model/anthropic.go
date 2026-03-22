package model

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AnthropicRequest represents the Anthropic Messages API request format
type AnthropicRequest struct {
	Model       string                   `json:"model"`
	MaxTokens   int                      `json:"max_tokens"`
	System      interface{}              `json:"system,omitempty"` // string or []content_block
	Messages    []map[string]interface{} `json:"messages"`
	Stream      bool                     `json:"stream"`
	Temperature *float64                 `json:"temperature,omitempty"`
	Tools       []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice  interface{}              `json:"tool_choice,omitempty"`
}

// AnthropicResponse represents the Anthropic Messages API non-streaming response
type AnthropicResponse struct {
	ID           string                   `json:"id"`
	Type         string                   `json:"type"`
	Role         string                   `json:"role"`
	Model        string                   `json:"model"`
	Content      []map[string]interface{} `json:"content"`
	StopReason   string                   `json:"stop_reason"`
	StopSequence *string                  `json:"stop_sequence"`
	Usage        AnthropicUsage           `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// GenerateMsgID generates a message ID with msg_ prefix
func GenerateMsgID() string {
	return "msg_" + uuid.New().String()[:24]
}

// ReturnAnthropicResponse sends a complete non-streaming Anthropic response
func ReturnAnthropicResponse(text string, model string, gc *gin.Context) error {
	resp := &AnthropicResponse{
		ID:   GenerateMsgID(),
		Type: "message",
		Role: "assistant",
		Model: model,
		Content: []map[string]interface{}{
			{"type": "text", "text": text},
		},
		StopReason:   "end_turn",
		StopSequence: nil,
		Usage: AnthropicUsage{
			InputTokens:  0,
			OutputTokens: 0,
		},
	}
	gc.JSON(200, resp)
	return nil
}

// SendAnthropicStreamEvent sends a single SSE event in Anthropic format
func SendAnthropicStreamEvent(eventType string, data interface{}, gc *gin.Context) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %v", err)
	}
	line := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(jsonBytes))
	gc.Writer.Write([]byte(line))
	gc.Writer.Flush()
	return nil
}
