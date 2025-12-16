package tools

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Parser 解析 AI 输出中的工具调用
type Parser struct{}

// NewParser 创建解析器
func NewParser() *Parser {
	return &Parser{}
}

// toolCallPattern 匹配工具调用的 JSON 块
var toolCallPatterns = []*regexp.Regexp{
	// 标准 JSON 块格式
	regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`),
	// 代码块格式
	regexp.MustCompile("(?s)```json\\s*\\n(\\{[^`]*?\"tool\"[^`]*?\\})\\s*\\n```"),
	regexp.MustCompile("(?s)```\\s*\\n(\\{[^`]*?\"tool\"[^`]*?\\})\\s*\\n```"),
	// 单行 JSON 格式
	regexp.MustCompile(`(\{"tool"\s*:\s*"[^"]+"\s*,\s*"[^}]+\})`),
}

// ParseToolCalls 从 AI 输出中解析工具调用
func (p *Parser) ParseToolCalls(output string) ([]ParsedToolCall, string) {
	var calls []ParsedToolCall
	remainingText := output

	for _, pattern := range toolCallPatterns {
		matches := pattern.FindAllStringSubmatch(output, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			jsonStr := match[1]
			var rawCall map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &rawCall); err != nil {
				continue
			}

			// 提取工具名称
			toolName := ""
			if name, ok := rawCall["tool"].(string); ok {
				toolName = name
			} else if name, ok := rawCall["name"].(string); ok {
				toolName = name
			}

			if toolName == "" {
				continue
			}

			// 提取输入参数
			input := make(map[string]interface{})
			if inp, ok := rawCall["input"].(map[string]interface{}); ok {
				input = inp
			} else {
				// 其他字段作为输入
				for k, v := range rawCall {
					if k != "tool" && k != "name" && k != "type" {
						input[k] = v
					}
				}
			}

			calls = append(calls, ParsedToolCall{
				Name:  toolName,
				Input: input,
			})

			// 从剩余文本中移除已解析的工具调用
			remainingText = strings.Replace(remainingText, match[0], "", 1)
		}
	}

	// 清理剩余文本
	remainingText = strings.TrimSpace(remainingText)

	return calls, remainingText
}

// GenerateToolPrompt 生成工具使用的系统提示
func GenerateToolPrompt(tools []ToolDefinition) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## 可用工具\n\n")
	sb.WriteString("当你需要执行操作时，请使用以下格式调用工具：\n\n")
	sb.WriteString("<tool_call>\n{\"tool\": \"工具名称\", \"参数名\": \"参数值\"}\n</tool_call>\n\n")
	sb.WriteString("可用的工具：\n\n")

	for _, tool := range tools {
		sb.WriteString("### ")
		sb.WriteString(tool.Name)
		sb.WriteString("\n")
		if tool.Description != "" {
			sb.WriteString(tool.Description)
			sb.WriteString("\n")
		}
		sb.WriteString("参数：\n")

		for name, prop := range tool.InputSchema.Properties {
			required := ""
			for _, r := range tool.InputSchema.Required {
				if r == name {
					required = " (必需)"
					break
				}
			}
			sb.WriteString("- `")
			sb.WriteString(name)
			sb.WriteString("`")
			sb.WriteString(required)
			sb.WriteString(": ")
			sb.WriteString(prop.Description)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("重要提示：\n")
	sb.WriteString("- 每次只调用一个工具\n")
	sb.WriteString("- 工具调用必须使用 <tool_call> 标签包裹\n")
	sb.WriteString("- 等待工具执行结果后再继续\n")

	return sb.String()
}

// IsToolCallResponse 检查输出是否包含工具调用
func (p *Parser) IsToolCallResponse(output string) bool {
	for _, pattern := range toolCallPatterns {
		if pattern.MatchString(output) {
			return true
		}
	}
	return false
}
