# Cursor2API

将 Cursor API 转换为 OpenAI/Anthropic 兼容格式的代理服务。

## 原理

本项目利用 [Cursor 文档页面](https://cursor.com/cn/docs) 提供的免费 AI 聊天功能，通过 `https://cursor.com/api/chat` 接口与后端通信。

**关键特点：**
- **无需登录** - 文档页面的 AI 聊天功能对所有访问者开放
- **无需 API Key** - 不需要 Cursor 账号或付费订阅
- **支持多模型** - 可使用 Claude、GPT、Gemini 等模型
- **纯 HTTP 方案** - 无需浏览器，资源占用低

## 技术架构

### 核心思路

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  客户端请求  │────▶│  cursor2api  │────▶│  Cursor API │
│ (OpenAI/    │     │  (代理转换)   │     │ /api/chat   │
│  Anthropic) │◀────│              │◀────│             │
└─────────────┘     └──────────────┘     └─────────────┘
```

### 1. TLS 指纹模拟

Cursor API 会检测请求的 TLS 指纹来判断是否为真实浏览器。本项目使用 [surf](https://github.com/enetx/surf) 库模拟 Chrome 浏览器的 TLS 特征：

```go
client := surf.NewClient().Builder().Impersonate().Chrome().Build()
```

同时模拟完整的 Chrome 请求头：
- `sec-ch-ua`: Chrome 版本信息
- `sec-ch-ua-platform`: 操作系统
- `sec-fetch-*`: 请求来源信息
- 等等...

### 2. x-is-human Token 机制

Cursor 使用 `x-is-human` 请求头进行人机验证。这个 token 由前端 JavaScript 计算生成，有效期约 25 分钟。

**Token 生成流程：**

```
1. 获取 Cursor 验证脚本 (c.js)
   GET https://cursor.com/xxx/xxx/c.js?...
   
2. 注入浏览器环境模拟 (env.js)
   - navigator, window, document 等 DOM API
   - WebGL 指纹信息
   
3. 执行脚本生成 token
   Node.js 运行组合后的 JS 代码
   
4. 返回 x-is-human token
```

### 3. Token 生成

每次 API 请求都会生成新的 x-is-human token，避免被 Cursor 检测到 token 重复使用。

### 4. 协议转换

将 OpenAI/Anthropic 格式转换为 Cursor 内部格式：

```json
// Anthropic 请求
{
  "model": "claude-3.5-sonnet",
  "messages": [{"role": "user", "content": "Hello"}]
}

// 转换为 Cursor 格式
{
  "model": "claude-3.5-sonnet",
  "id": "abc123",
  "trigger": "submit-message",
  "messages": [{
    "role": "user",
    "id": "xyz789",
    "parts": [{"type": "text", "text": "Hello"}]
  }]
}
```

### 5. Tool Use 实现

由于 Cursor 文档页 AI 不原生支持工具调用，通过 **Prompt 注入** 方式实现：

```
1. 请求带有 tools 定义
   ↓
2. 将工具定义注入到第一条用户消息
   "你有以下工具可用: bash, read_file, write_file..."
   ↓
3. AI 按照提示格式输出工具调用
   <tool_calls>[{"name":"bash","arguments":{"command":"ls"}}]</tool_calls>
   ↓
4. 解析响应，转换为标准 tool_use 格式返回
```

## 功能特性

- **Anthropic Messages API** - 完整支持 `/v1/messages` 接口
- **OpenAI Chat API** - 支持 `/v1/chat/completions` 接口
- **流式响应** - 支持 SSE 流式输出
- **纯 HTTP 实现** - 无需浏览器，内存占用低
- **TLS 指纹模拟** - 模拟真实浏览器特征
- **Tool Use 协议** - 支持 Anthropic 工具调用协议

## 项目结构

```
cursor2api/
├── cmd/server/          # 程序入口
│   └── main.go
├── internal/            # 内部包
│   ├── client/          # Cursor API 客户端 (TLS 指纹模拟)
│   ├── config/          # 配置管理
│   ├── handler/         # HTTP 处理器 (Anthropic/OpenAI 协议)
│   ├── token/           # Token 生成 (x-is-human)
│   ├── toolify/         # Tool Use 协议 (Prompt 注入 + 解析)
│   └── logger/          # 日志模块
├── jscode/              # JS 脚本
│   ├── env.js           # 浏览器环境模拟
│   └── main.js          # Token 生成入口
├── static/              # 静态文件
├── config.yaml          # 配置文件
└── README.md
```

## 快速开始

### 前置要求

1. **获取浏览器指纹** - 在浏览器控制台运行以下脚本：

```javascript
function getBrowserFingerprint() {
  const canvas = document.createElement('canvas');
  const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl');
  let unmaskedVendor = '', unmaskedRenderer = '';
  if (gl) {
    const debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
    if (debugInfo) {
      unmaskedVendor = gl.getParameter(debugInfo.UNMASKED_VENDOR_WEBGL) || '';
      unmaskedRenderer = gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL) || '';
    }
  }
  return btoa(JSON.stringify({
    "UNMASKED_VENDOR_WEBGL": unmaskedVendor,
    "UNMASKED_RENDERER_WEBGL": unmaskedRenderer,
    "userAgent": navigator.userAgent
  }));
}
console.log('FP:', getBrowserFingerprint());
```

2. **获取 ScriptURL** - 访问 https://cursor.com/cn/docs，打开开发者工具网络面板，找到类似 `https://cursor.com/xxx/xxx/c.js?...` 的请求 URL

3. **下载 env.js** - 参考 `jscode/README.md` 下载必要的 JS 文件

### Docker 部署 (推荐)

```bash
docker-compose up -d
```

或者使用环境变量：

```bash
docker run -d -p 3010:3010 \
  -e FP="你的base64指纹" \
  -e SCRIPT_URL="https://cursor.com/xxx/xxx/c.js?..." \
  cursor2api
```

### 本地运行

```bash
# 安装依赖
go mod tidy

# 下载 env.js
curl -o jscode/env.js https://raw.githubusercontent.com/jhhgiyv/cursorweb2api/master/jscode/env.js

# 编译
go build -o cursor2api ./cmd/server

# 运行
./cursor2api
```

服务默认运行在 `http://localhost:3010`

## 配置

编辑 `config.yaml`：

```yaml
# 服务端口
port: 3010

# 请求超时（秒）
timeout: 60

# 代理设置（可选）
# proxy: "http://127.0.0.1:7890"

# Cursor 验证脚本 URL（必须配置）
script_url: "https://cursor.com/xxx/xxx/c.js?i=0&v=3&h=cursor.com"

# 外部 token 计算服务（可选，如果不配置则使用本地 Node.js）
# x_is_human_server_url: ""

# 浏览器指纹配置
fingerprint:
  unmasked_vendor_webgl: "Google Inc. (Intel)"
  unmasked_renderer_webgl: "ANGLE (Intel, Intel(R) UHD Graphics ...)"
  user_agent: "Mozilla/5.0 ..."

# 支持的模型列表
models: "gpt-4o,claude-3.5-sonnet,claude-3.7-sonnet"
```

支持的环境变量：
- `PORT` - 服务端口
- `PROXY` - 代理地址
- `SCRIPT_URL` - Cursor 验证脚本 URL
- `FP` - 浏览器指纹（base64 编码的 JSON）
- `MODELS` - 模型列表

## API 接口

### Anthropic Messages API

```bash
curl http://localhost:3010/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: any" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

### OpenAI Chat API

```bash
curl http://localhost:3010/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello"}],
    "stream": true
  }'
```

### 其他接口

- `GET /v1/models` - 获取模型列表
- `GET /health` - 健康检查
- `GET /status` - 客户端状态（token 是否有效）

## Claude Code 集成

```bash
# 设置 API 地址
export ANTHROPIC_BASE_URL=http://localhost:3010

# 运行 Claude Code
claude
```

## 支持的模型

所有请求统一映射到 `claude-opus-4-5-20251101`。

## 依赖

- Go 1.21+
- Node.js（用于生成 x-is-human token）

## 免责声明

本项目仅供学习和研究目的使用。

- 本项目是一个非官方的第三方工具，与 Cursor 官方无任何关联
- 使用本项目可能违反 Cursor 的服务条款，请自行承担风险
- 本项目不提供任何形式的担保，包括但不限于适销性、特定用途适用性
- 作者不对使用本项目造成的任何直接或间接损失负责
- 请勿将本项目用于商业用途或任何违法活动

使用本项目即表示您已阅读并同意以上声明。

## 许可证

MIT
