package handler

import (
	"net/http"

	"cursor2api/internal/tools"

	"github.com/gin-gonic/gin"
)

// ToolExecuteRequest 工具执行请求
type ToolExecuteRequest struct {
	ToolName string                 `json:"tool_name"`
	Input    map[string]interface{} `json:"input"`
	WorkDir  string                 `json:"work_dir,omitempty"`
}

// ToolExecuteResponse 工具执行响应
type ToolExecuteResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// ExecuteTool 执行工具调用（供本地测试使用）
func ExecuteTool(c *gin.Context) {
	var req ToolExecuteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ToolExecuteResponse{
			Success: false,
			Error:   "无效的请求格式: " + err.Error(),
		})
		return
	}

	executor := tools.NewExecutor()
	if req.WorkDir != "" {
		executor.SetWorkDir(req.WorkDir)
	}

	output, err := executor.Execute(req.ToolName, req.Input)
	if err != nil {
		c.JSON(http.StatusOK, ToolExecuteResponse{
			Success: false,
			Output:  output,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ToolExecuteResponse{
		Success: true,
		Output:  output,
	})
}

// ListTools 列出可用工具
func ListTools(c *gin.Context) {
	toolList := []tools.ToolDefinition{
		{
			Name:        "bash",
			Description: "执行 bash 命令",
			InputSchema: tools.InputSchema{
				Type: "object",
				Properties: map[string]tools.Property{
					"command": {Type: "string", Description: "要执行的命令"},
					"cwd":     {Type: "string", Description: "工作目录（可选）"},
				},
				Required: []string{"command"},
			},
		},
		{
			Name:        "read_file",
			Description: "读取文件内容",
			InputSchema: tools.InputSchema{
				Type: "object",
				Properties: map[string]tools.Property{
					"path": {Type: "string", Description: "文件路径"},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "write_file",
			Description: "写入文件",
			InputSchema: tools.InputSchema{
				Type: "object",
				Properties: map[string]tools.Property{
					"path":    {Type: "string", Description: "文件路径"},
					"content": {Type: "string", Description: "文件内容"},
				},
				Required: []string{"path", "content"},
			},
		},
		{
			Name:        "list_dir",
			Description: "列出目录内容",
			InputSchema: tools.InputSchema{
				Type: "object",
				Properties: map[string]tools.Property{
					"path": {Type: "string", Description: "目录路径"},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "edit",
			Description: "编辑文件（查找替换）",
			InputSchema: tools.InputSchema{
				Type: "object",
				Properties: map[string]tools.Property{
					"path":        {Type: "string", Description: "文件路径"},
					"old_string":  {Type: "string", Description: "要替换的内容"},
					"new_string":  {Type: "string", Description: "替换后的内容"},
					"replace_all": {Type: "boolean", Description: "是否替换所有匹配"},
				},
				Required: []string{"path", "old_string", "new_string"},
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"tools": toolList,
	})
}
