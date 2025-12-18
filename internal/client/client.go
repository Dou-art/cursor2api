// Package client 提供 Cursor API 客户端实现
// 使用 surf 库模拟 Chrome TLS 指纹发送请求
package client

import (
	"fmt"
	"sync"

	"cursor2api/internal/config"
	"cursor2api/internal/logger"
	"cursor2api/internal/token"

	"github.com/enetx/g"
	"github.com/enetx/surf"
)

var log = logger.Get().WithPrefix("Client")

// Cursor API 端点
const cursorChatAPI = "https://cursor.com/api/chat"

// Chrome 浏览器请求头模拟
var chromeChatHeaders = map[string]string{
	"Content-Type":               "application/json",
	"sec-ch-ua-platform":         `"Windows"`,
	"x-path":                     "/api/chat",
	"sec-ch-ua":                  `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`,
	"x-method":                   "POST",
	"sec-ch-ua-bitness":          `"64"`,
	"sec-ch-ua-mobile":           "?0",
	"sec-ch-ua-arch":             `"x86"`,
	"sec-ch-ua-platform-version": `"19.0.0"`,
	"origin":                     "https://cursor.com",
	"sec-fetch-site":             "same-origin",
	"sec-fetch-mode":             "cors",
	"sec-fetch-dest":             "empty",
	"referer":                    "https://cursor.com/en-US/learn/how-ai-models-work",
	"accept-language":            "zh-CN,zh;q=0.9,en;q=0.8",
	"priority":                   "u=1, i",
}

// Service HTTP 客户端服务
type Service struct {
	surfClient *surf.Client
	cfg        *config.Config
}

var (
	instance *Service
	once     sync.Once
)

// GetService 获取服务单例
func GetService() *Service {
	once.Do(func() {
		instance = &Service{
			cfg: config.Get(),
		}
		instance.init()
	})
	return instance
}

// init 初始化 HTTP 客户端
func (s *Service) init() {
	s.surfClient = surf.NewClient().
		Builder().
		Impersonate().
		Chrome().
		Build()

	log.Info("客户端初始化完成")
}

// GetXIsHuman 获取当前 token（兼容旧接口）
func (s *Service) GetXIsHuman() string {
	return s.GetXIsHumanForKey("")
}

// GetXIsHumanForKey 获取指定 API Key 的 token
func (s *Service) GetXIsHumanForKey(apiKey string) string {
	t, err := token.GetPool().GetToken(apiKey)
	if err != nil {
		log.Error("获取 token 失败: %v", err)
		return ""
	}
	return t
}

// CursorChatRequest Cursor API 请求格式
type CursorChatRequest struct {
	Context  []CursorContext `json:"context,omitempty"`
	Model    string          `json:"model"`
	ID       string          `json:"id"`
	Messages []CursorMessage `json:"messages"`
	Trigger  string          `json:"trigger"`
}

// CursorContext 上下文信息
type CursorContext struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	FilePath string `json:"filePath"`
}

// CursorMessage 消息格式
type CursorMessage struct {
	Parts []CursorPart `json:"parts"`
	ID    string       `json:"id,omitempty"`
	Role  string       `json:"role"`
}

// CursorPart 消息内容
type CursorPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SendRequest 发送非流式请求
func (s *Service) SendRequest(req CursorChatRequest) (string, error) {
	return s.SendRequestWithIP(req, "")
}

// SendRequestWithIP 发送非流式请求（带客户端 IP）
func (s *Service) SendRequestWithIP(req CursorChatRequest, clientIP string) (string, error) {
	return s.doRequest(req, nil, clientIP)
}

// SendStreamRequest 发送流式请求
func (s *Service) SendStreamRequest(req CursorChatRequest, onChunk func(chunk string)) error {
	return s.SendStreamRequestWithIP(req, onChunk, "")
}

// SendStreamRequestWithIP 发送流式请求（带客户端 IP）
func (s *Service) SendStreamRequestWithIP(req CursorChatRequest, onChunk func(chunk string), clientIP string) error {
	_, err := s.doRequest(req, onChunk, clientIP)
	return err
}

// doRequest 发送 API 请求
func (s *Service) doRequest(req CursorChatRequest, onChunk func(chunk string), clientIP string) (string, error) {
	headers := s.buildChatHeaders(clientIP)

	log.Debug("发送请求到 Cursor API: model=%s", req.Model)

	resp := s.surfClient.Post(g.String(cursorChatAPI), req).SetHeaders(headers).Do()
	if resp.IsErr() {
		log.Error("Cursor API 请求失败: %v", resp.Err())
		return "", fmt.Errorf("请求失败: %w", resp.Err())
	}

	r := resp.Ok()
	if r.StatusCode != 200 {
		body := string(r.Body.String())
		log.Error("Cursor API 返回错误: HTTP %d, 响应: %s", r.StatusCode, body)
		return "", fmt.Errorf("HTTP %d: %s", r.StatusCode, body)
	}

	bodyStr := string(r.Body.String())
	if onChunk != nil {
		onChunk(bodyStr)
	}

	log.Debug("Cursor API 响应成功, 长度: %d", len(bodyStr))
	return bodyStr, nil
}

// buildChatHeaders 构建聊天请求头
func (s *Service) buildChatHeaders(clientIP string) map[string]string {
	headers := make(map[string]string, len(chromeChatHeaders)+3)
	for k, v := range chromeChatHeaders {
		headers[k] = v
	}
	headers["x-is-human"] = s.GetXIsHuman()
	// 转发客户端 IP
	if clientIP != "" {
		headers["X-Forwarded-For"] = clientIP
		headers["X-Real-IP"] = clientIP
		log.Debug("转发客户端 IP: %s", clientIP)
	}
	return headers
}
