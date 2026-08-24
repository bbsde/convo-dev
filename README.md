# convo-dev — Convo 插件开发包

为 [Convo](https://github.com/bbsde/convo)（多平台大模型聚合网关）开发 wasm 插件所需的一切：**pdk SDK + 项目脚手架 + 契约文档**。

## 快速开始

```bash
git clone https://github.com/bbsde/convo-dev.git
cd convo-dev

# 生成一个新插件项目（OpenAI 兼容平台接入模板）
./new-plugin.sh my-plugin ~/src/my-plugin

cd ~/src/my-plugin
go mod tidy
tinygo build -target wasi -no-debug -o my-plugin.wasm .

# 安装到 convo 网关调试：见 docs/testing.md
```

前置：[Go ≥ 1.22](https://go.dev/dl/) + [TinyGo](https://tinygo.org/getting-started/install/) + [convo 网关](https://github.com/bbsde/convo/releases)。

## 插件能做什么

- **接入新平台**：实现 `/v1/chat/completions`（真流式）、`/v1/embeddings`、`/v1/images/generations`、`/v1/audio/speech`、`/v1/videos` 任意组合——把任意 LLM 平台变成 Convo 的 OpenAI 兼容上游，自动获得多账号轮询、failover、熔断、用量统计。
- **自带管理 UI**：单 HTML 内嵌 wasm，出现在 Convo 控制台里（添加账号、验证、状态展示）。
- **自建数据表**：声明式 SQLite 迁移 + 通用账号表契约。
- **通用 action RPC**：UI / 定时任务 / 账号级调用的自定义入口。

## 仓库结构

| 路径 | 说明 |
|---|---|
| `pdk/` | 插件 SDK（Go module `github.com/bbsde/convo-dev/pdk`），wasm↔host 的全部 ABI 封装 |
| `template/` | 插件模板：最小可编译的 OpenAI 兼容接入（echo + verify_key + chat 流式/非流式中继 + UI + migration） |
| `new-plugin.sh` | 脚手架：从模板生成插件项目并替换占位符 |
| `docs/manifest.md` | plugin.json 字段参考（表/模型/整形/定时任务/授权） |
| `docs/pdk-api.md` | SDK API 参考（注册回调 / HTTP / 会话 / 配置与状态） |
| `docs/rules.md` | **硬性约定与坑**（echo 必须、禁 sync 原语、禁 JSON 树化、UI 绝对 URL…） |
| `docs/testing.md` | 编译、安装调试、打包分发、发布自查 |

## 先读 rules.md

里面每条约定都对应真实事故（wasm 运行期崩溃、线性内存打穿、内置浏览器 404……）。脚手架模板已按约定写好，改写时别把防护改没了。

## 分发

- **自由插件**：无 `license` 字段，打包 `.cpk` 即可分发。
- **付费插件**：上传 Convo 插件市场定价销售，license 由市场在线签发（Ed25519 验签，绑定 wasm 指纹）。
