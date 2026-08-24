# 插件开发硬性约定（坑都是实打实踩过的）

违反任何一条都可能导致**运行期崩溃 / 内存打穿 / 发布被冒烟测试拦下**。改动前先读本文。

## 1. `echo` action 必须实现

原样回传 payload、返回 200：

```go
case "echo":
    pdk.SetResponse(200, payload)
    return nil
```

发布冒烟测试（加载全部插件 + echo 实调）靠它验证接线，缺了直接过不了发布关。

## 2. wasm 内禁用一切 sync 原语

`sync.Once` / `sync.Mutex` / `sync.WaitGroup`——**初始化期和运行期调用都会崩**（TinyGo 下 nilPanic）：
- 包级初始化踩过（包级 `var x = uuidV4()` 触发 Once）
- 运行期也踩过（成功路径里的 `once.Do` → 流式全 502）

并发控制由 host 的实例池兜底，wasm 单线程。需要「只做一次」就包级字符串判空懒初始化：

```go
var sessionID string // 判空懒初始化，别用 sync.Once
```

## 3. 不做整份请求体 JSON 树化

`json.Unmarshal(body, &m)` 其中 `m map[string]any` → `json.Marshal(m)`——TinyGo 堆会膨胀 10~20 倍，大上下文 agent 请求**两度打穿 128MB/512MB 线性内存上限**（线性内存只增不减）。

正确姿势：
- **OpenAI 透传中继类**：manifest 声明 `chat_request_patch`（host 进 wasm 前整形），插件读 `pdk.ChatPatchedKey` 非空即 body 零解析透传；流式意图改读 `pdk.ClientStreamKey`。
- **必须本地改写时**：顶层 `map[string]json.RawMessage` 浅解码，只动要改的键，其余 `RawMessage` 原样透传（见 template/main.go）。

## 4. 插件 UI 的 API 调用一律绝对 URL

```js
var ABS = window.__CONVO_ABS__ || (location.origin + (location.pathname.split("/plugins/")[0] || ""));
fetch(ABS + "/api/plugins/<name>/action", {...})
```

根相对 `"/api/..."` 在 ZCode 内置浏览器（Electron IAB）下会被重定基（深层页面 `/app/<id>/**` 之下必坏）、普通浏览器下也可能 404。host 注入的 `__CONVO_ABS__` = origin + 网关前缀，是唯一可靠锚点。

## 5. 表名与插件名

- 表名必须 `^[A-Za-z_][A-Za-z0-9_]{0,62}$`（host 拼 SQL，不合规拒绝加载）。
- 插件名全局唯一（也是 provider 族名），脚手架约束 `^[a-z][a-z0-9_]{1,30}$`。

## 6. 账号表标准列契约

用 `tables.accounts` 声明 + migration 建表（template/migrations/1.sql 已含标准列）：
`id / external_id / credentials / display / enabled / expires_at / last_used_at / error_count / disabled_until / created_at / updated_at`。
- `credentials`：verify_key 组装的 JSON 字符串（chat 时从 `combos[0].key` 还原）。
- `external_id`：账号唯一外部标识（重复添加 upsert 合并）。

## 7. 错误语义

- chat 里账号失效回 **401/403**（host 记账号错误、计数、熔断、failover 下一个账号）。
- 客户端断开（`pdk.IsCanceled(err)`）**静默收尾**——往死连接写 502 只会留噪声日志。
- 流式上游非 2xx：先 `Stream.Status()` peek，读错误体回 `SetResponse`，**别把错误体当 SSE emit**。

## 8. 分发：自由插件 vs 付费插件

- **自由插件**：`plugin.json` 无 `license` 字段，免签直接分发（.cpk = `plugin.json + <name>.wasm` 的 tar.gz）。
- **付费插件**：`license` 块由市场（market）按购买记录在线签发（Ed25519，签名绑定 `name+customer+expires+sha256(wasm)`）；convo 侧编译期注入公钥验签，验不过不加载。**改了 wasm 字节旧 license 即失效**（sha256 变了），需重新签发。

## 9. 会话与状态

- 跨请求状态用 `pdk.SetKV`（host 落库持久化），别依赖 wasm 内存（实例池多实例，内存不保证归属）。
- 多步登录（发码→验证→续期）用 `Session*` API 保持 cookie jar；登录会话按插件名分桶，跨两次 action 的会话句柄不因实例池失效。
