// Package token 提供 Token 池管理
package token

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cursor2api/internal/config"
	"cursor2api/internal/logger"

	"github.com/enetx/g"
	"github.com/enetx/surf"
)

var log = logger.Get().WithPrefix("TokenPool")

// Pool Token 池管理器
type Pool struct {
	tokens     map[string]*TokenEntry // name -> token
	nameMap    map[string]string      // apiKey -> name (用于显示)
	roundRobin []*TokenEntry          // 轮询 token 池
	rrIndex    int32                  // 轮询索引
	client     *surf.Client
	cfg        *config.Config
	mu         sync.RWMutex
	envJS      string
	mainJS     string
	stopChan   chan struct{}
	nextID     int32 // 用于生成 token 名称
	hitCount   int64 // 缓存命中次数
	missCount  int64 // 缓存未命中次数
	poolSize   int   // 轮询池大小
}

// TokenEntry Token 条目
type TokenEntry struct {
	Name      string // token 名称，如 "Token-1", "Token-2"
	Token     string
	CreatedAt time.Time
	UseCount  int64 // 使用次数
	mu        sync.Mutex
}

const (
	tokenExpiry     = 25 * time.Minute // token 有效期
	refreshInterval = 20 * time.Minute // 刷新间隔（提前5分钟刷新）
)

var (
	instance *Pool
	once     sync.Once
)

// GetPool 获取 Token 池单例
func GetPool() *Pool {
	once.Do(func() {
		cfg := config.Get()
		poolSize := cfg.TokenPoolSize
		if poolSize <= 0 {
			poolSize = 3 // 默认 3 个 token 轮询
		}
		instance = &Pool{
			tokens:     make(map[string]*TokenEntry),
			nameMap:    make(map[string]string),
			roundRobin: make([]*TokenEntry, 0, poolSize),
			cfg:        cfg,
			poolSize:   poolSize,
		}
		instance.init()
	})
	return instance
}

func (p *Pool) init() {
	// 初始化 HTTP 客户端
	p.client = surf.NewClient().Builder().Impersonate().Chrome().Build()
	p.stopChan = make(chan struct{})

	// 加载 JS 模板
	envJS, err := os.ReadFile("jscode/env.js")
	if err != nil {
		log.Warn("failed to load env.js: %v", err)
	}
	p.envJS = string(envJS)

	mainJS, err := os.ReadFile("jscode/main.js")
	if err != nil {
		log.Warn("failed to load main.js: %v", err)
	}
	p.mainJS = string(mainJS)

	// 预生成轮询 token 池
	log.Info("预热 %d 个 token...", p.poolSize)
	for i := 0; i < p.poolSize; i++ {
		tokenStr, err := p.generateToken()
		if err != nil {
			log.Error("预热 token %d 失败: %v", i+1, err)
			continue
		}
		name := p.generateName()
		entry := &TokenEntry{
			Name:      name,
			Token:     tokenStr,
			CreatedAt: time.Now(),
		}
		p.roundRobin = append(p.roundRobin, entry)
		log.Info("预热 %s 完成 (%d/%d)", name, i+1, p.poolSize)
	}

	// 启动后台刷新协程
	go p.backgroundRefresh()

	log.Info("Initialized (轮询池: %d)", len(p.roundRobin))
}

// preWarmToken 预热 token
func (p *Pool) preWarmToken(apiKey string) {
	tokenStr, err := p.generateToken()
	if err != nil {
		log.Error("Pre-warm failed: %v", err)
		return
	}

	name := p.generateName()
	p.mu.Lock()
	p.tokens[apiKey] = &TokenEntry{
		Name:      name,
		Token:     tokenStr,
		CreatedAt: time.Now(),
	}
	p.nameMap[apiKey] = name
	p.mu.Unlock()

	log.Info("Pre-warmed %s for key: %s", name, truncateKey(apiKey))
}

// generateName 生成 token 名称
func (p *Pool) generateName() string {
	id := atomic.AddInt32(&p.nextID, 1)
	return fmt.Sprintf("Token-%d", id)
}

// backgroundRefresh 后台定时刷新所有 token
func (p *Pool) backgroundRefresh() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.refreshAllTokens()
		case <-p.stopChan:
			return
		}
	}
}

// refreshAllTokens 刷新轮询池中的所有 token
func (p *Pool) refreshAllTokens() {
	p.mu.RLock()
	poolLen := len(p.roundRobin)
	p.mu.RUnlock()

	for i := 0; i < poolLen; i++ {
		p.refreshRoundRobinToken(i)
	}

	log.Info("后台刷新完成 (轮询池: %d)", poolLen)
}

// GetToken 获取 Token（每次生成新 token）
func (p *Pool) GetToken(apiKey string) (string, error) {
	// 每次请求生成新 token，避免被 Cursor 检测到重复使用
	log.Debug("生成新 token...")
	tokenStr, err := p.generateToken()
	if err != nil {
		log.Error("生成 token 失败: %v", err)
		return "", err
	}
	atomic.AddInt64(&p.hitCount, 1)
	log.Debug("新 token 生成成功")
	return tokenStr, nil
}

// refreshRoundRobinToken 刷新轮询池中指定索引的 token
func (p *Pool) refreshRoundRobinToken(idx int) {
	p.mu.RLock()
	if idx >= len(p.roundRobin) {
		p.mu.RUnlock()
		return
	}
	entry := p.roundRobin[idx]
	p.mu.RUnlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// 双重检查
	if time.Since(entry.CreatedAt) < tokenExpiry {
		return
	}

	tokenStr, err := p.generateToken()
	if err != nil {
		log.Error("刷新 %s 失败: %v", entry.Name, err)
		return
	}

	entry.Token = tokenStr
	entry.CreatedAt = time.Now()
	log.Info("刷新 %s 完成", entry.Name)
}

// refreshToken 刷新指定 API Key 的 Token
func (p *Pool) refreshToken(apiKey string) (string, error) {
	p.mu.Lock()
	entry, exists := p.tokens[apiKey]
	if !exists {
		name := p.generateName()
		entry = &TokenEntry{Name: name}
		p.tokens[apiKey] = entry
		p.nameMap[apiKey] = name
	}
	p.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// 双重检查
	if time.Since(entry.CreatedAt) < tokenExpiry && entry.Token != "" {
		return entry.Token, nil
	}

	tokenStr, err := p.generateToken()
	if err != nil {
		return "", err
	}

	entry.Token = tokenStr
	entry.CreatedAt = time.Now()

	log.Info("Created %s for key: %s (total: %d)", entry.Name, truncateKey(apiKey), p.Count())
	return tokenStr, nil
}

// Count 返回 token 总数
func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tokens)
}

// Stats 返回统计信息
func (p *Pool) Stats() (total int, hits, misses int64) {
	p.mu.RLock()
	total = len(p.tokens)
	p.mu.RUnlock()
	hits = atomic.LoadInt64(&p.hitCount)
	misses = atomic.LoadInt64(&p.missCount)
	return
}

// List 返回所有 token 信息
func (p *Pool) List() []map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]map[string]any, 0, len(p.tokens))
	for key, entry := range p.tokens {
		result = append(result, map[string]any{
			"name":    entry.Name,
			"key":     truncateKey(key),
			"uses":    entry.UseCount,
			"age":     time.Since(entry.CreatedAt).Round(time.Second).String(),
			"expires": (tokenExpiry - time.Since(entry.CreatedAt)).Round(time.Second).String(),
		})
	}
	return result
}

// generateToken 使用 Node.js 生成 token
func (p *Pool) generateToken() (string, error) {
	if p.cfg.ScriptURL == "" {
		return "", fmt.Errorf("script_url not configured")
	}

	// 获取 Cursor 脚本
	cursorJS, err := p.fetchCursorScript()
	if err != nil {
		return "", fmt.Errorf("fetch cursor script: %w", err)
	}

	// 构建 JS 代码
	code := p.buildJSCode(cursorJS)

	// 写入临时文件执行（避免 argument list too long）
	tmpFile, err := os.CreateTemp("", "cursor_token_*.js")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(code); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// 使用 Node.js 执行临时文件
	cmd := exec.Command("node", tmpPath)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("node error: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("execute node: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// fetchCursorScript 获取 Cursor 验证脚本
func (p *Pool) fetchCursorScript() (string, error) {
	headers := map[string]string{
		"sec-ch-ua-arch":             `"x86"`,
		"sec-ch-ua-platform":         `"Windows"`,
		"sec-ch-ua":                  `"Chromium";v="140", "Not=A?Brand";v="24", "Google Chrome";v="140"`,
		"sec-ch-ua-bitness":          `"64"`,
		"sec-ch-ua-mobile":           "?0",
		"sec-ch-ua-platform-version": `"19.0.0"`,
		"sec-fetch-site":             "same-origin",
		"sec-fetch-mode":             "no-cors",
		"sec-fetch-dest":             "script",
		"referer":                    "https://cursor.com/",
		"accept-language":            "zh-CN,zh;q=0.9,en;q=0.8",
	}

	resp := p.client.Get(g.String(p.cfg.ScriptURL)).SetHeaders(headers).Do()
	if resp.IsErr() {
		return "", fmt.Errorf("fetch script: %w", resp.Err())
	}

	return string(resp.Ok().Body.String()), nil
}

// buildJSCode 构建 JavaScript 代码
func (p *Pool) buildJSCode(cursorJS string) string {
	fp := p.cfg.Fingerprint
	replacer := strings.NewReplacer(
		"$$currentScriptSrc$$", p.cfg.ScriptURL,
		"$$UNMASKED_VENDOR_WEBGL$$", fp.UnmaskedVendorWebGL,
		"$$UNMASKED_RENDERER_WEBGL$$", fp.UnmaskedRendererWebGL,
		"$$userAgent$$", fp.UserAgent,
		"$$env_jscode$$", p.envJS,
		"$$cursor_jscode$$", cursorJS,
	)
	return replacer.Replace(p.mainJS)
}

// Close 关闭 Token 池
func (p *Pool) Close() {
	close(p.stopChan)
}

func truncateKey(s string) string {
	if s == "default" {
		return "default"
	}
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}
