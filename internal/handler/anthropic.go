// Package handler 提供 HTTP 请求处理器
// 包含 Anthropic Messages API 兼容的处理函数
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cursor2api/internal/client"
	"cursor2api/internal/toolify"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 注意: log 变量在 openai.go 中定义

// ================== 请求/响应结构体 ==================

// MessagesRequest Anthropic Messages API 请求格式
type MessagesRequest struct {
	Model     string                   `json:"model"`
	Messages  []Message                `json:"messages"`
	MaxTokens int                      `json:"max_tokens"`
	Stream    bool                     `json:"stream"`
	System    interface{}              `json:"system,omitempty"` // 可以是 string 或 []ContentBlock
	Tools     []toolify.ToolDefinition `json:"tools,omitempty"`
}

// Message 消息格式
type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // 可以是 string 或 []ContentBlock
}

// MessagesResponse Anthropic Messages API 响应格式
type MessagesResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

// ContentBlock 内容块
type ContentBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`    // tool_use
	Name  string                 `json:"name,omitempty"`  // tool_use
	Input map[string]interface{} `json:"input,omitempty"` // tool_use
}

// Usage token 使用统计
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// CursorSSEEvent Cursor SSE 事件格式
type CursorSSEEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
}

// ================== 辅助函数 ==================

// generateID 生成唯一标识符
func generateID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
}

// getTextContent 从 interface{} 提取文本内容
// 支持 string 和 []ContentBlock 两种格式
func getTextContent(content interface{}) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				if block["type"] == "text" {
					if text, ok := block["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// mapModelName 将模型名称映射到 Cursor 支持的格式
func mapModelName(model string) string {
	// 直接透传模型名称，不做转换
	if model == "" {
		return "claude-opus-4-5-20251101"
	}
	return model
}

// ================== 处理器函数 ==================

// CountTokens 估算 token 数量
func CountTokens(c *gin.Context) {
	var req MessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	// 简单估算：每 4 个字符约 1 个 token
	totalChars := len(getTextContent(req.System))
	for _, msg := range req.Messages {
		totalChars += len(getTextContent(msg.Content))
	}
	tokens := totalChars / 4
	if tokens < 1 {
		tokens = 1
	}

	c.JSON(http.StatusOK, gin.H{"input_tokens": tokens})
}

// getClientIP 获取客户端真实 IP
func getClientIP(c *gin.Context) string {
	// 优先从 X-Forwarded-For 获取
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// 取第一个 IP（最原始的客户端）
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// 其次从 X-Real-IP 获取
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return xri
	}
	// 最后用 RemoteAddr
	return c.ClientIP()
}

// Messages 处理 Anthropic Messages API 请求
func Messages(c *gin.Context) {
	// 记录请求 Headers
	log.Debug("[Anthropic] ========== 请求开始 ==========")
	log.Debug("[Anthropic] 请求路径: %s", c.Request.URL.String())
	log.Debug("[Anthropic] 请求头:")
	for key, values := range c.Request.Header {
		log.Debug("  %s: %s", key, strings.Join(values, ", "))
	}

	var req MessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Error("[Anthropic] 解析请求失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	// 记录请求参数
	log.Info("[Anthropic] 请求参数:")
	log.Info("  模型: %s", req.Model)
	log.Info("  消息数: %d", len(req.Messages))
	log.Info("  最大Token: %d", req.MaxTokens)
	log.Info("  流式: %v", req.Stream)
	if len(req.Tools) > 0 {
		log.Info("  工具数: %d", len(req.Tools))
	}

	// 记录消息内容
	for i, msg := range req.Messages {
		content := getTextContent(msg.Content)
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		log.Debug("  消息[%d] 角色=%s 内容=%s", i, msg.Role, content)
	}

	// 转换为 Cursor 请求格式
	cursorReq := convertToCursor(req)
	clientIP := getClientIP(c)
	log.Debug("[Anthropic] 客户端 IP: %s", clientIP)

	if req.Stream {
		handleStream(c, cursorReq, req.Model, req.Tools, clientIP)
	} else {
		handleNonStream(c, cursorReq, req.Model, req.Tools, clientIP)
	}
}

// ================== 请求转换 ==================

// convertToCursor 将 Anthropic 请求转换为 Cursor 格式
func convertToCursor(req MessagesRequest) client.CursorChatRequest {
	messages := make([]client.CursorMessage, 0, len(req.Messages)+1)

	// 构建系统消息
	sysText := getTextContent(req.System)
	if sysText != "" {
		messages = append(messages, client.CursorMessage{
			Parts: []client.CursorPart{{Type: "text", Text: sysText}},
			ID:    generateID(),
			Role:  "system",
		})
	}

	// 检测是否有 tool_result（表示工具已执行过）
	hasToolResult := false
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			if content, ok := msg.Content.([]interface{}); ok {
				for _, item := range content {
					if block, ok := item.(map[string]interface{}); ok {
						if block["type"] == "tool_result" {
							hasToolResult = true
							break
						}
					}
				}
			}
		}
	}

	// 只有第一次调用时才注入工具提示（没有 tool_result）
	toolPrompt := ""
	if len(req.Tools) > 0 && !hasToolResult {
		toolPrompt = toolify.GenerateToolPrompt(req.Tools)
		log.Info("[Anthropic] 注入工具提示词, 长度: %d, 工具数: %d", len(toolPrompt), len(req.Tools))
		log.Debug("[Anthropic] 工具提示词内容:\n%s", toolPrompt)
	} else if len(req.Tools) > 0 && hasToolResult {
		log.Debug("[Anthropic] 跳过工具提示词注入 (已有 tool_result)")
	}

	// 添加用户/助手消息
	firstUserMsg := true
	for _, msg := range req.Messages {
		text := extractMessageText(msg)
		if text != "" {
			// 把工具提示放在第一条用户消息前面
			if msg.Role == "user" && firstUserMsg && toolPrompt != "" {
				log.Debug("[Anthropic] 工具提示词已注入到第一条用户消息")
				text = toolPrompt + "\n\n" + text
				firstUserMsg = false
			}
			messages = append(messages, client.CursorMessage{
				Parts: []client.CursorPart{{Type: "text", Text: text}},
				ID:    generateID(),
				Role:  msg.Role,
			})
		}
	}

	return client.CursorChatRequest{
		Model:    mapModelName(req.Model),
		ID:       generateID(),
		Messages: messages,
		Trigger:  "submit-message",
	}
}

// extractMessageText 从消息中提取文本
func extractMessageText(msg Message) string {
	content := msg.Content
	if content == nil {
		return ""
	}

	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			switch block["type"] {
			case "text":
				if text, ok := block["text"].(string); ok {
					texts = append(texts, text)
				}
			case "tool_result":
				// 提取 tool_result 内容
				toolID := ""
				if id, ok := block["tool_use_id"].(string); ok {
					toolID = id
				}
				resultContent := ""
				if c, ok := block["content"].(string); ok {
					resultContent = c
				} else if c, ok := block["content"].([]interface{}); ok {
					for _, item := range c {
						if b, ok := item.(map[string]interface{}); ok {
							if b["type"] == "text" {
								if t, ok := b["text"].(string); ok {
									resultContent += t
								}
							}
						}
					}
				}
				texts = append(texts, fmt.Sprintf("[Tool %s result]: %s", toolID, resultContent))
			}
		}
		return strings.Join(texts, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ================== API 处理 ==================

// handleStream 处理流式请求
func handleStream(c *gin.Context, cursorReq client.CursorChatRequest, model string, tools []toolify.ToolDefinition, clientIP string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, _ := c.Writer.(http.Flusher)
	id := "msg_" + generateID()

	// 发送 message_start
	_, _ = c.Writer.WriteString("event: message_start\n")
	_, _ = fmt.Fprintf(c.Writer, `data: {"type":"message_start","message":{"id":"%s","type":"message","role":"assistant","content":[],"model":"%s","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":0}}}`+"\n\n", id, model)
	flusher.Flush()

	var buffer, fullResponse strings.Builder
	blockIndex := 0
	toolCount := 0

	// 发送工具调用的辅助函数
	sendToolCall := func(toolName, argsJSON string) {
		toolID := fmt.Sprintf("toolu_%d", toolCount)
		toolCount++

		var args map[string]any
		_ = json.Unmarshal([]byte(argsJSON), &args)
		inputJSON, _ := json.Marshal(args)
		partialJSONStr, _ := json.Marshal(string(inputJSON))

		_, _ = c.Writer.WriteString("event: content_block_start\n")
		_, _ = fmt.Fprintf(c.Writer, `data: {"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"%s","name":"%s","input":{}}}`+"\n\n", blockIndex, toolID, toolName)
		_, _ = c.Writer.WriteString("event: content_block_delta\n")
		_, _ = fmt.Fprintf(c.Writer, `data: {"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":%s}}`+"\n\n", blockIndex, string(partialJSONStr))
		_, _ = c.Writer.WriteString("event: content_block_stop\n")
		_, _ = fmt.Fprintf(c.Writer, `data: {"type":"content_block_stop","index":%d}`+"\n\n", blockIndex)
		blockIndex++
		flusher.Flush()
	}

	// 标记是否已发送文本块开始
	textBlockStarted := false

	svc := client.GetService()
	err := svc.SendStreamRequestWithIP(cursorReq, func(chunk string) {
		buffer.WriteString(chunk)
		content := buffer.String()
		lines := strings.Split(content, "\n")

		if !strings.HasSuffix(content, "\n") && len(lines) > 0 {
			buffer.Reset()
			buffer.WriteString(lines[len(lines)-1])
			lines = lines[:len(lines)-1]
		} else {
			buffer.Reset()
		}

		for _, line := range lines {
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			var event CursorSSEEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			if event.Type == "text-delta" && event.Delta != "" {
				fullResponse.WriteString(event.Delta)

				// 实时发送文本块
				if !textBlockStarted {
					_, _ = c.Writer.WriteString("event: content_block_start\n")
					_, _ = fmt.Fprintf(c.Writer, `data: {"type":"content_block_start","index":%d,"content_block":{"type":"text","text":""}}`+"\n\n", blockIndex)
					textBlockStarted = true
				}

				textJSON, _ := json.Marshal(event.Delta)
				_, _ = c.Writer.WriteString("event: content_block_delta\n")
				_, _ = fmt.Fprintf(c.Writer, `data: {"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":%s}}`+"\n\n", blockIndex, string(textJSON))
				flusher.Flush()
			}
		}
	}, clientIP)

	if err != nil {
		_, _ = c.Writer.WriteString("event: error\n")
		_, _ = c.Writer.WriteString(`data: {"type":"error","error":{"message":"` + err.Error() + `"}}` + "\n\n")
		flusher.Flush()
		return
	}

	// 结束文本块
	if textBlockStarted {
		_, _ = c.Writer.WriteString("event: content_block_stop\n")
		_, _ = fmt.Fprintf(c.Writer, `data: {"type":"content_block_stop","index":%d}`+"\n\n", blockIndex)
		flusher.Flush()
		blockIndex++
	}

	// 解析完整响应检查工具调用
	responseText := fullResponse.String()
	toolCalls, _ := toolify.ParseToolCalls(responseText)

	// 发送工具调用
	stopReason := "end_turn"
	if len(toolCalls) > 0 {
		stopReason = "tool_use"
		for _, call := range toolCalls {
			sendToolCall(call.Function.Name, call.Function.Arguments)
		}
	}

	_, _ = c.Writer.WriteString("event: message_delta\n")
	_, _ = fmt.Fprintf(c.Writer, `data: {"type":"message_delta","delta":{"stop_reason":"%s","stop_sequence":null},"usage":{"output_tokens":100}}`+"\n\n", stopReason)
	_, _ = c.Writer.WriteString("event: message_stop\n")
	_, _ = c.Writer.WriteString(`data: {"type":"message_stop"}` + "\n\n")
	flusher.Flush()
}

// handleNonStream 处理非流式请求
func handleNonStream(c *gin.Context, cursorReq client.CursorChatRequest, model string, tools []toolify.ToolDefinition, clientIP string) {
	svc := client.GetService()
	result, err := svc.SendRequestWithIP(cursorReq, clientIP)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	// 解析响应
	var fullText strings.Builder
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}

		var event CursorSSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "text-delta" && event.Delta != "" {
			fullText.WriteString(event.Delta)
		}
	}

	responseText := fullText.String()
	var contentBlocks []ContentBlock
	stopReason := "end_turn"

	// 检测工具调用
	if len(tools) > 0 {
		toolCalls, cleanText := toolify.ParseToolCalls(responseText)
		if len(toolCalls) > 0 {
			stopReason = "tool_use"
			if cleanText != "" {
				contentBlocks = append(contentBlocks, ContentBlock{Type: "text", Text: cleanText})
			}
			for _, call := range toolCalls {
				var args map[string]any
				_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
				contentBlocks = append(contentBlocks, ContentBlock{
					Type:  "tool_use",
					ID:    "toolu_" + call.ID,
					Name:  call.Function.Name,
					Input: args,
				})
			}
		} else {
			contentBlocks = append(contentBlocks, ContentBlock{Type: "text", Text: responseText})
		}
	} else {
		contentBlocks = append(contentBlocks, ContentBlock{Type: "text", Text: responseText})
	}

	c.JSON(http.StatusOK, MessagesResponse{
		ID:         "msg_" + generateID(),
		Type:       "message",
		Role:       "assistant",
		Content:    contentBlocks,
		Model:      model,
		StopReason: stopReason,
		Usage:      Usage{InputTokens: 100, OutputTokens: 100},
	})
}
