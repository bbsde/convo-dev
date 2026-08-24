# convo-dev — Convo 插件开发脚手架

为 [Convo](https://github.com/bbsde/convo)（多平台大模型聚合网关）开发 wasm 插件所需的一切：**`convo-dev` 脚手架 CLI + pdk SDK + 契约文档**。

## 快速开始

```bash
# 安装脚手架命令（一次性）
go install github.com/bbsde/convo-dev@latest

# 生成插件项目（远程拉取本仓模板，无需 clone）
convo-dev init my_plugin
cd my_plugin
./build.sh          # TinyGo 编译（wasm→src/）+ cpk 打包（→dist/）
```

前置：[Go ≥ 1.22](https://go.dev/dl/) + [TinyGo](https://tinygo.org/getting-started/install/) + [convo 网关](https://github.com/bbsde/convo/releases)。

更新：`convo-dev update`（查 GitHub 最新 tag 并自更新；新 tag 推出后立即可更，不等代理索引）。

测试：`convo-dev test`（插件项目根目录跑）——静态检查 + 自动下载 convo 网关（缓存 `~/.convo-dev/gateway/`）→ 临时实例安装 `.cpk` → echo 实调，测完自动清理；`CONVO_DEV_GATEWAY=<路径>` 可指定本地网关。

`init` 生成的项目结构：

```
my_plugin/
├── build.sh        # 构建脚本（wasm→src/，cpk→dist/）
├── dist/           # 分发包（my_plugin-<ver>.cpk；wasm 编译到 src/ 供目录加载）
├── docs/           # 项目文档（README 骨架 + NOTES 逆向笔记模板）
└── src/            # 源码
    ├── main.go     # OpenAI 兼容接入模板（echo + verify_key + chat 流式/非流式中继）
    ├── plugin.json # manifest（模型清单/表声明/chat_request_patch）
    ├── go.mod
    ├── migrations/ # SQLite 迁移（通用账号表标准列）
    └── ui/         # 插件管理 UI（添加账号全流程）
```

`convo-dev init` 选项：`-d <dir>` 目标目录、`--ref <tag|分支>` 拉指定版本模板（默认 main）、
`--url <tarball>` 镜像地址、`--template <dir>` 本地模板（离线）。`convo-dev version` 看版本。

## 插件能做什么

- **接入新平台**：实现 `/v1/chat/completions`（真流式）、`/v1/embeddings`、`/v1/images/generations`、`/v1/audio/speech`、`/v1/videos` 任意组合——把任意 LLM 平台变成 Convo 的 OpenAI 兼容上游，自动获得多账号轮询、failover、熔断、用量统计。
- **自带管理 UI**：单 HTML 内嵌 wasm，出现在 Convo 控制台里（添加账号、验证、状态展示）。
- **自建数据表**：声明式 SQLite 迁移 + 通用账号表契约。
- **通用 action RPC**：UI / 定时任务 / 账号级调用的自定义入口。

## 仓库结构

| 路径 | 说明 |
|---|---|
| `main.go` + `go.mod` | `convo-dev` CLI 本体（`go install github.com/bbsde/convo-dev@latest`） |
| `pdk/` | 插件 SDK（独立 Go module `github.com/bbsde/convo-dev/pdk`），wasm↔host 的全部 ABI 封装 |
| `template/` | 项目模板（CLI 远程拉取的源）：`src/` 源码 + `AGENTS.md` + `build.sh` + `docs/`（契约文档四篇 + README/NOTES 骨架，**随 init 进入每个插件项目**） |
| `docs/releasing.md` | 发布与版本管理（CLI/pdk 双 tag 序列、模板版本对齐、pdk 升级检查单）——面向本仓维护者，不随项目分发 |

## 先读 template/docs/rules.md

里面每条约定都对应真实事故（wasm 运行期崩溃、线性内存打穿、内置浏览器 404……）。脚手架模板已按约定写好，改写时别把防护改没了。（`convo-dev init` 后它就在你项目的 `docs/rules.md`。）

## 分发

- **自由插件**：无 `license` 字段，打包 `.cpk` 即可分发。
- **付费插件**：上传 Convo 插件市场定价销售，license 由市场在线签发（Ed25519 验签，绑定 wasm 指纹）。

## 许可

[MIT](LICENSE) © 2026 bbsde
