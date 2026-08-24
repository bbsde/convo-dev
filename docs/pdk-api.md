# pdk API 参考

`pdk` 是 convo 插件开发包（TinyGo 编译到 wasm）。

```go
import pdk "github.com/bbsde/convo-dev/pdk"
```

编译：`tinygo build -target wasi -no-debug -o <name>.wasm .`

## 回调注册（main() 里调用）

| 函数 | 说明 |
|---|---|
| `RegisterChat(fn func(req, combos []byte) error)` | `/v1/chat/completions` 入口。`req`=OpenAI ChatRequest JSON（透传，自行反序列化）；`combos`=`[{key, model}]` JSON（host 已按轮询/failover 排序，`key`=账号 credentials JSON） |
| `RegisterEmbed / RegisterImage / RegisterSpeech / RegisterVideo(fn)` | 对应 `/v1/embeddings`、`/v1/images/generations`、`/v1/audio/speech`、`/v1/videos`。按需实现，不实现就不导出（host 探测后该端点不可用）。video 是异步任务制：req 按 `"_op"`（无/`get`/`cancel`）分发 |
| `HandleAction(fn func(action string, payload []byte) error)` | 通用插件 RPC：UI、定时器、账号级调用统一入口。action 名插件自定义，host 不解读 |
| `RegisterAsset(name string, data []byte)` | 内嵌资产（如 `"ui"`、`"migration_1"`），配合 `//go:embed` 实现「plugin.json + 单 wasm」分发 |

**`echo` action 是硬性约定**：原样回传 payload（200）——发布冒烟测试依赖它。

## 输出与配置

| 函数 | 说明 |
|---|---|
| `SetResponse(status int, body []byte)` | 非流式响应（chat 401/403 会被 host 记账号错误/熔断） |
| `Emit(b []byte)` | 写一段流式输出（OpenAI-SSE 字节，可多次） |
| `Log(level int, msg string)` | 日志；level 0=debug 1=info 2=warn 3=error |
| `ReportUsage(model, key string)` | 回告实际使用的 model/key（配额统计） |
| `GetConfig(key string) string` | 读插件配置（host 实例化时注入）。key 以 `*` 结尾做前缀枚举（返回键值对 JSON 对象文本） |
| `SetKV(key, value string)` | 写插件运行时状态（host 落 plugin_state 表，不解读）。键 ≤128B、值 ≤64KB；建议带自用前缀（如 `"st:"`） |

## HTTP

| 函数 | 说明 |
|---|---|
| `HTTPRequest(method, url, headers map[string]string, body []byte) (*HTTPResponse, error)` | 非流式请求；headers 为 map（内部 JSON 序列化传 host） |
| `HTTPStreamOpen(...) (*Stream, error)` | 流式请求；`Stream.Next()` 逐块读（EOF 时 `eof=true`）、`Stream.Status()` 首读前 peek 状态码（非 2xx 别当 SSE 透传）、`Stream.Close()` |
| `SessionOpen() / SessionRequest(session, method, url, headers, body, follow) / SessionClose(session)` | 有状态会话（cookie jar 跨请求保持；`follow=false` 不跟随重定向，可读 3xx 本身）。多步登录/续期编排用 |
| `SessionCookies(session, url) / SessionSetCookies(session, url, cookies)` | 导出/预置会话 cookie（`"k=v; k=v2"` 形态） |
| `ResponseStatus / ResponseBody / ResponseHeader / ResponseClose(handle)` | 响应句柄低阶读（仅对 SessionRequest 返回的句柄有意义；HTTPRequest 已自动关闭） |

## 辅助

| 常量/函数 | 说明 |
|---|---|
| `ChatPatchedKey` / `ClientStreamKey` | host 声明式整形后的保留配置键：`ChatPatchedKey` 非空 = body 已整形可直接透传；`ClientStreamKey`（`"true"/"false"`）= 客户端真实流式意图（`force_stream` 后 body 的 stream 恒 true）。老 host 不注入——须保留回退 |
| `IsCanceled(err error) bool` | 调用方是否取消（客户端断开）。取消时静默收尾，别写 502 |

## 内存与并发约束（重要）

- **wasm 单线程**：并发由 host 的实例池兜底；**禁用一切 sync 原语**（`sync.Once`/`Mutex`/`WaitGroup` 初始化期和运行期调用都会崩）。「只做一次」用包级字符串判空懒初始化。
- host→插件输入走 **8MB 静态缓冲**（单块或阶段合计超限返回 413）；线性内存只增不减，**不要整份树化大 JSON**（decode→Marshal 会使 TinyGo 堆膨胀 10~20 倍）。
- TinyGo 标准库可用性以实际编译为准；`encoding/json`、`strings`、`strconv`、`time`、`unsafe.Slice` 均已验证可用。
