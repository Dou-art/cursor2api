// Package tools 提供工具调用解析和执行功能
package tools

// ToolDefinition Anthropic 工具定义格式
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema InputSchema `json:"input_schema"`
}

// InputSchema JSON Schema 格式的输入参数定义
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property JSON Schema 属性定义
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// ToolUse AI 返回的工具调用
type ToolUse struct {
	Type  string                 `json:"type"` // "tool_use"
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}

// ToolResult 工具执行结果
type ToolResult struct {
	Type      string      `json:"type"` // "tool_result"
	ToolUseID string      `json:"tool_use_id"`
	Content   interface{} `json:"content"` // string 或 []ContentBlock
	IsError   bool        `json:"is_error,omitempty"`
}

// ContentBlock 内容块（用于 tool_result）
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ParsedToolCall 从 AI 输出解析的工具调用
type ParsedToolCall struct {
	Name  string                 `json:"name"`
	Input map[string]interface{} `json:"input"`
}
