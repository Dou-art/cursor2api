# jscode 目录

本目录包含用于计算 `x-is-human` token 的 JavaScript 文件。

## 文件说明

- `main.js` - 主入口文件（已包含）
- `env.js` - 浏览器环境模拟文件（需要下载）

## 获取 env.js

`env.js` 文件较大（约 336KB），需要从 cursorweb2api 项目下载：

```bash
curl -o env.js https://raw.githubusercontent.com/jhhgiyv/cursorweb2api/master/jscode/env.js
```

或者手动下载：https://github.com/jhhgiyv/cursorweb2api/blob/master/jscode/env.js

## 依赖

使用本地 Node.js 计算 token 需要安装 Node.js：

```bash
# 检查 Node.js 是否安装
node --version
```

如果不想安装 Node.js，可以配置 `x_is_human_server_url` 使用外部服务计算 token。
