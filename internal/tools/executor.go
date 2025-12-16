package tools

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Executor 工具执行器
type Executor struct {
	workDir     string
	allowedDirs []string
	timeout     time.Duration
}

// NewExecutor 创建工具执行器
func NewExecutor() *Executor {
	homeDir, _ := os.UserHomeDir()
	return &Executor{
		workDir:     homeDir,
		allowedDirs: []string{homeDir, "/tmp"},
		timeout:     30 * time.Second,
	}
}

// SetWorkDir 设置工作目录
func (e *Executor) SetWorkDir(dir string) {
	e.workDir = dir
}

// Execute 执行工具调用
func (e *Executor) Execute(toolName string, input map[string]interface{}) (string, error) {
	switch toolName {
	case "bash", "run_command":
		return e.executeBash(input)
	case "read_file":
		return e.readFile(input)
	case "write_file", "write_to_file":
		return e.writeFile(input)
	case "list_dir", "list_directory":
		return e.listDir(input)
	case "edit", "str_replace_editor":
		return e.editFile(input)
	default:
		return "", fmt.Errorf("未知工具: %s", toolName)
	}
}

// executeBash 执行 bash 命令
func (e *Executor) executeBash(input map[string]interface{}) (string, error) {
	command, ok := input["command"].(string)
	if !ok {
		// 尝试其他字段名
		if cmd, ok := input["CommandLine"].(string); ok {
			command = cmd
		} else {
			return "", fmt.Errorf("缺少 command 参数")
		}
	}

	// 获取工作目录
	cwd := e.workDir
	if dir, ok := input["cwd"].(string); ok && dir != "" {
		cwd = dir
	} else if dir, ok := input["Cwd"].(string); ok && dir != "" {
		cwd = dir
	}

	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 设置超时
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}
		if err != nil {
			return output, fmt.Errorf("命令执行失败: %v\n%s", err, output)
		}
		return output, nil
	case <-time.After(e.timeout):
		cmd.Process.Kill()
		return "", fmt.Errorf("命令执行超时 (%v)", e.timeout)
	}
}

// readFile 读取文件
func (e *Executor) readFile(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		if p, ok := input["file_path"].(string); ok {
			path = p
		} else {
			return "", fmt.Errorf("缺少 path 参数")
		}
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workDir, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	return string(content), nil
}

// writeFile 写入文件
func (e *Executor) writeFile(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		if p, ok := input["file_path"].(string); ok {
			path = p
		} else if p, ok := input["TargetFile"].(string); ok {
			path = p
		} else {
			return "", fmt.Errorf("缺少 path 参数")
		}
	}

	content, ok := input["content"].(string)
	if !ok {
		if c, ok := input["CodeContent"].(string); ok {
			content = c
		} else {
			return "", fmt.Errorf("缺少 content 参数")
		}
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workDir, path)
	}

	// 创建目录
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return fmt.Sprintf("已写入文件: %s (%d 字节)", path, len(content)), nil
}

// listDir 列出目录内容
func (e *Executor) listDir(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		if p, ok := input["DirectoryPath"].(string); ok {
			path = p
		} else {
			path = e.workDir
		}
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workDir, path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("读取目录失败: %v", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("目录: %s\n\n", path))

	for _, entry := range entries {
		info, _ := entry.Info()
		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("[DIR]  %s/\n", entry.Name()))
		} else {
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			result.WriteString(fmt.Sprintf("[FILE] %s (%d bytes)\n", entry.Name(), size))
		}
	}

	return result.String(), nil
}

// editFile 编辑文件（查找替换）
func (e *Executor) editFile(input map[string]interface{}) (string, error) {
	path, ok := input["path"].(string)
	if !ok {
		if p, ok := input["file_path"].(string); ok {
			path = p
		} else {
			return "", fmt.Errorf("缺少 path 参数")
		}
	}

	oldStr, _ := input["old_string"].(string)
	newStr, _ := input["new_string"].(string)

	if oldStr == "" {
		return "", fmt.Errorf("缺少 old_string 参数")
	}

	// 处理相对路径
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.workDir, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	original := string(content)
	if !strings.Contains(original, oldStr) {
		return "", fmt.Errorf("未找到要替换的内容")
	}

	// 替换
	replaceAll := false
	if ra, ok := input["replace_all"].(bool); ok {
		replaceAll = ra
	}

	var modified string
	if replaceAll {
		modified = strings.ReplaceAll(original, oldStr, newStr)
	} else {
		modified = strings.Replace(original, oldStr, newStr, 1)
	}

	if err := os.WriteFile(path, []byte(modified), 0644); err != nil {
		return "", fmt.Errorf("写入文件失败: %v", err)
	}

	return fmt.Sprintf("已编辑文件: %s", path), nil
}
