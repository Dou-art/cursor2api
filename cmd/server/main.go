// Cursor2API - 将 Cursor API 转换为 OpenAI/Anthropic 兼容格式
//
// 本项目通过浏览器自动化技术调用 Cursor 的 AI 接口，
// 并将其转换为标准的 OpenAI 和 Anthropic API 格式，
// 使得各种 AI 客户端可以直接使用 Cursor 的服务。
package main

import (
	"cursor2api/internal/client"
	"cursor2api/internal/config"
	"cursor2api/internal/handler"
	"cursor2api/internal/logger"
	"cursor2api/internal/token"

	"github.com/gin-gonic/gin"
)

var log = logger.Get().WithPrefix("Main")

func main() {
	// 加载配置
	cfg := config.Get()

	// 初始化 Token Pool（预热 token，确保启动时就准备好）
	log.Info("正在初始化 Token Pool...")
	token.GetPool()

	// 初始化 HTTP 客户端服务
	log.Info("正在初始化客户端服务...")
	client.GetService()

	// 创建 Gin 引擎
	r := gin.Default()

	// ==================== 路由配置 ====================

	// OpenAI 兼容接口
	r.GET("/v1/models", handler.ListModels)
	r.POST("/v1/chat/completions", handler.ChatCompletions)

	// Anthropic Messages API 兼容接口
	r.POST("/v1/messages", handler.Messages)
	r.POST("/messages", handler.Messages)
	r.POST("/v1/messages/count_tokens", handler.CountTokens)
	r.POST("/messages/count_tokens", handler.CountTokens)

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// 客户端状态
	r.GET("/status", func(c *gin.Context) {
		svc := client.GetService()
		hasToken := svc.GetXIsHuman() != ""
		c.JSON(200, gin.H{"hasToken": hasToken})
	})

	// 静态文件
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	// 启动服务
	log.Info("服务运行在端口 %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Error("启动失败: %v", err)
	}
}
