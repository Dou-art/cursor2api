// Package config 提供配置文件加载和管理功能
package config

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构
type Config struct {
	// Port 服务监听端口
	Port string `yaml:"port"`
	// Timeout 请求超时时间（秒）
	Timeout int `yaml:"timeout"`
	// Proxy 代理地址
	Proxy string `yaml:"proxy"`
	// ScriptURL Cursor 验证脚本 URL
	ScriptURL string `yaml:"script_url"`
	// XIsHumanServerURL 外部 token 计算服务地址
	XIsHumanServerURL string `yaml:"x_is_human_server_url"`
	// Fingerprint 浏览器指纹配置
	Fingerprint FingerprintConfig `yaml:"fingerprint"`
	// Models 支持的模型列表
	Models string `yaml:"models"`
	// TokenPoolSize Token 轮询池大小
	TokenPoolSize int `yaml:"token_pool_size"`
}

// FingerprintConfig 浏览器指纹配置
type FingerprintConfig struct {
	// UnmaskedVendorWebGL WebGL 厂商
	UnmaskedVendorWebGL string `yaml:"unmasked_vendor_webgl"`
	// UnmaskedRendererWebGL WebGL 渲染器
	UnmaskedRendererWebGL string `yaml:"unmasked_renderer_webgl"`
	// UserAgent 用户代理
	UserAgent string `yaml:"user_agent"`
}

var (
	cfg  *Config
	once sync.Once
)

// Get 获取全局配置实例（单例模式）
func Get() *Config {
	once.Do(func() {
		cfg = &Config{
			Port:    "3010",
			Timeout: 60,
			Models:  "gpt-4o,claude-3.5-sonnet,claude-3.7-sonnet",
			Fingerprint: FingerprintConfig{
				UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36",
			},
		}
		load(cfg)
	})
	return cfg
}

// load 从配置文件和环境变量加载配置
func load(c *Config) {
	// 尝试读取 YAML 配置文件
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		log.Printf("[配置] 未找到 config.yaml，使用默认配置")
	} else {
		if err := yaml.Unmarshal(data, c); err != nil {
			log.Printf("[配置] 解析 config.yaml 失败: %v", err)
		} else {
			log.Printf("[配置] 已加载 config.yaml")
		}
	}

	// 环境变量覆盖配置文件
	if port := os.Getenv("PORT"); port != "" {
		c.Port = port
	}
	if proxy := os.Getenv("PROXY"); proxy != "" {
		c.Proxy = proxy
	}
	if scriptURL := os.Getenv("SCRIPT_URL"); scriptURL != "" {
		c.ScriptURL = scriptURL
	}
	if xIsHumanServerURL := os.Getenv("X_IS_HUMAN_SERVER_URL"); xIsHumanServerURL != "" {
		c.XIsHumanServerURL = xIsHumanServerURL
	}
	if fp := os.Getenv("FP"); fp != "" {
		// FP 是 base64 编码的 JSON
		if decoded, err := base64.StdEncoding.DecodeString(fp); err == nil {
			var fpConfig FingerprintConfig
			if err := json.Unmarshal(decoded, &fpConfig); err == nil {
				c.Fingerprint = fpConfig
			}
		}
	}
	if models := os.Getenv("MODELS"); models != "" {
		c.Models = models
	}

	// 输出最终配置
	log.Printf("[配置] 端口: %s, 超时: %ds", c.Port, c.Timeout)
	if c.ScriptURL != "" {
		log.Printf("[配置] ScriptURL: %s", c.ScriptURL)
	}
	if c.XIsHumanServerURL != "" {
		log.Printf("[配置] XIsHumanServerURL: %s", c.XIsHumanServerURL)
	}
}
