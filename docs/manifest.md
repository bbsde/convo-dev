# plugin.json（manifest）字段参考

manifest 是插件与 host（convo 网关）之间的声明契约，随 `plugin.json` 与 wasm 同目录分发。

## 顶层字段

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | ✓ | 插件名，全局唯一。规则 `^[a-z][a-z0-9_]{1,30}$`（小写开头，兼容表名前缀） |
| `title` | | 展示名（空则前端回退用 name） |
| `abi_version` | ✓ | 必须等于 host 支持的 ABI 版本（当前为 `1`） |
| `module` | ✓ | wasm 文件名（相对 manifest 目录），约定与 name 同名：`<name>.wasm` |
| `tables` | | 声明插件自建 SQLite 表（见下） |
| `models` | | 静态模型清单（见下） |
| `schedules` | | 定时任务声明（host 调度器按此触发插件 action） |
| `features` | | 插件自声明开关（`map[string]bool`），经 `GetConfig` 读回 |
| `chat_request_patch` | | host 侧 chat 请求体声明式整形（见下）——透传类插件强烈建议声明 |
| `hooks` | | 生命周期钩子 action 名：`on_load`、`on_account_added` |
| `account_selection` | | 账号选择策略声明（可选） |
| `endpoints` | | 展示用端点声明；事实源是 loader 探测 wasm 的 `handle_<name>` 导出 |
| `description` / `author` / `version` | | 元信息（`version` 仅展示，建议语义化） |
| `schema_version` | | 插件 schema 版本：`>=1` 走版本化迁移（`migration_N` 资产按序应用）；`0`/缺省 = 旧式整文件 apply-or-skip |
| `license` | | 付费插件授权块（由市场签发，见分发说明）。**无此字段 = 自由插件免签** |

## tables

```json
"tables": {
  "accounts": "myplugin_accounts",
  "info": "myplugin_info"
}
```

- 表名必须是合法 SQL 标识符（`^[A-Za-z_][A-Za-z0-9_]{0,62}$`）——host 会把它拼进 SQL，不合规直接拒绝加载。
- `accounts`：账号表。host 按「通用账号表标准列契约」CRUD（标准列见 template/migrations/1.sql），`credentials`/`display` 对 host 不透明（插件自管 JSON）。
- `info`：apikey 类插件的信息表（如 base_url 配置、自定义平台记录）。

## models

```json
"models": [
  { "id": "glm-5.3", "name": "GLM-5.3", "endpoints": ["chat"], "capabilities": ["tools", "reasoning"], "max_context": 1000000 }
]
```

- `id`：对外模型名（客户端请求里的 `model` 就用它；可用「插件名:模型名」或裸名路由）。
- `endpoints`：该模型支持的端点短名（`chat`/`embed`/`image`/`speech`/`video`），缺省 `["chat"]`。
- `capabilities`：能力标签（展示/路由参考，如 `tools`、`reasoning`）。
- `max_context`：tokens 上限，0/缺省 = 未知。

## chat_request_patch

声明后 host（convo ≥ 1.0.69）在请求体进入 wasm **之前**于 Go 侧整形，插件读到 `pdk.ChatPatchedKey` 非空即 body 零解析透传：

| 键 | 作用 |
|---|---|
| `model_from_combo` | 用 combos 选中的上游模型名替换 body 的 `model` |
| `force_stream` | 强制上游流式（host 端按客户端意图解流；body 的 `stream` 恒 true） |
| `stream_options_include_usage` | 流式请求注入 `stream_options.include_usage`（host 统计 tokens 用） |
| `content_filter_rules_key` | 指向内容过滤规则设置键（host 注入过滤上下文） |

**必须保留本地浅解码回退**（老 host 不注入 `ChatPatchedKey`；参照 template/main.go 的 `handleChat`）。

## schedules

```json
"schedules": [ { "action": "refresh", "interval": "1h" } ]
```

- `action`：定时触发的插件 action 名（经 `HandleAction` 回调）。
- `interval`：固定间隔（`"30m"`/`"1h"`/`"2h"`…）；或 `daily_at_setting` 指向存 `"HH:MM"` 的设置键做每日定时；`enabled_setting` 可选指向存 `"true"/"false"` 的开关键。
