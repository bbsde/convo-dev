// Package pdk 是 convo 网关的插件开发包（TinyGo）。
//
// 插件作者 import pdk "github.com/bbsde/convo-dev/pdk"，在 main() 里调用
// RegisterChat 等注册回调，用 tinygo 编译到 wasm（-target wasi）即可投放插件目录。
// 脚手架与文档见仓库根（new-plugin.sh / docs/）。
//
// 内存模型：
//   - host→插件 输入（req/combos 字节）：host 调用本包导出的 alloc，写入一块静态缓冲；
//     插件用 readBytes(ptr,len) 按 绝对地址 取视图。
//   - 插件→host 数据（URL/header/body 等字节）：取 Go 切片底层数组地址 ptrLen(b) 传入。
//
// 所有 convo.* host 函数经 //go:wasmimport 声明（无函数体，由宿主提供）。
package pdk

import (
	"encoding/json"
	"errors"
	"strings"
	"unsafe"
)

// ---- host 函数导入（签名须与宿主 convo 模块一致）----

//go:wasmimport convo log
func hostLog(level, ptr, length uint32)

//go:wasmimport convo set_response
func hostSetResponse(status, ptr, length uint32)

//go:wasmimport convo emit
func hostEmit(ptr, length uint32)

//go:wasmimport convo report_usage
func hostReportUsage(modelPtr, modelLen, keyPtr, keyLen uint32)

//go:wasmimport convo get_config
func hostGetConfig(keyPtr, keyLen, outPtr, outLen uint32) uint32

//go:wasmimport convo set_kv
func hostSetKV(keyPtr, keyLen, valPtr, valLen uint32)

//go:wasmimport convo http_request
func hostHTTPRequest(mPtr, mLen, urlPtr, urlLen, hPtr, hLen, bPtr, bLen uint32) uint32

//go:wasmimport convo response_status
func hostResponseStatus(handle uint32) uint32

//go:wasmimport convo response_body_len
func hostResponseBodyLen(handle uint32) uint32

//go:wasmimport convo response_body
func hostResponseBody(handle, outPtr, outLen uint32) uint32

//go:wasmimport convo response_close
func hostResponseClose(handle uint32)

//go:wasmimport convo http_stream_open
func hostHTTPStreamOpen(mPtr, mLen, urlPtr, urlLen, hPtr, hLen, bPtr, bLen uint32) uint32

//go:wasmimport convo stream_next
func hostStreamNext(handle, outPtr, outLen uint32) uint32

//go:wasmimport convo stream_close
func hostStreamClose(handle uint32)

//go:wasmimport convo last_error_len
func hostLastErrorLen() uint32

//go:wasmimport convo last_error
func hostLastError(outPtr, outLen uint32) uint32

//go:wasmimport convo response_header
func hostResponseHeader(handle, namePtr, nameLen, outPtr, outLen uint32) uint32

//go:wasmimport convo http_session_open
func hostSessionOpen() uint32

//go:wasmimport convo http_session_request
func hostSessionRequest(session, mPtr, mLen, urlPtr, urlLen, hPtr, hLen, bPtr, bLen, follow uint32) uint32

//go:wasmimport convo http_session_close
func hostSessionClose(session uint32)

//go:wasmimport convo http_session_cookies
func hostSessionCookies(session, urlPtr, urlLen, outPtr, outLen uint32) uint32

//go:wasmimport convo http_session_set_cookies
func hostSessionSetCookies(session, urlPtr, urlLen, cookiesPtr, cookiesLen uint32) uint32

// ---- 内嵌资产（ui / schema 等）：插件 main() 里 RegisterAsset 注册，
// host 经 asset_len / asset_ptr 导出按名读取（零拷贝：直接给线性内存中内嵌字节的地址）。
// 用于把 ui/index.html、schema.sql 内嵌进 wasm，实现"plugin.json + 单 wasm"分发。
var assets = map[string][]byte{}

// RegisterAsset 注册一段内嵌资产。name 如 "ui"、"schema"。
// 须在 main() 中调用（host 实例化 wasm 跑 _start 时即生效）。
func RegisterAsset(name string, data []byte) { assets[name] = data }

func assetLookup(namePtr, nameLen uint32) ([]byte, bool) {
	b, ok := assets[string(readBytes(namePtr, nameLen))]
	return b, ok
}

//export asset_len
func assetLen(namePtr, nameLen uint32) uint32 {
	b, ok := assetLookup(namePtr, nameLen)
	if !ok {
		return 0xffffffff // 资产名未注册
	}
	return uint32(len(b))
}

//export asset_ptr
func assetPtr(namePtr, nameLen uint32) uint32 {
	b, ok := assetLookup(namePtr, nameLen)
	if !ok || len(b) == 0 {
		return 0 // 不存在或空资产：无可用地址
	}
	return uint32(uintptr(unsafe.Pointer(&b[0])))
}

// ---- 静态缓冲：承载 host→插件 输入 ----

var (
	heap         [8 << 20]byte // 8MB 静态缓冲，专供 host 写入输入（req+combos 等；内联 base64 图像请求可达数 MB）
	heapOff      uint32
	heapOverflow bool // 本调用阶段缓冲溢出（alloc 检测；handle_* 入口消费并拒绝调用）
)

//export alloc
func alloc(size uint32) uint32 {
	if size == 0 {
		return 0
	}
	if heapOff == 0 {
		heapOverflow = false // 新阶段首个分配：清除上一阶段的陈旧溢出标志
	}
	if size > uint32(len(heap)) {
		// 单块即超缓冲（如巨型内联图像请求）：返回一个必然越界的地址
		// （[4GB-size, 4GB)，超出任何 wasm32 线性内存），host 的 Memory.Write
		// 会失败并干净报错——绝不把数据写进插件堆造成静默 corruption。
		Log(2, "pdk: input 单块超过静态缓冲 8MB 上限，拒绝")
		return ^uint32(0) - (size - 1)
	}
	end := heapOff + size
	if end > uint32(len(heap)) {
		// 阶段内放不下（req+combos 合计超限）：回卷会覆盖尚未消费的前序输入——
		// 置溢出标志由 handle_* 入口统一拒绝（413），而不是静默覆盖。
		heapOverflow = true
		heapOff = 0
		Log(2, "pdk: input 合计超过静态缓冲 8MB 上限，拒绝")
		return uint32(uintptr(unsafe.Pointer(&heap[0])))
	}
	off := heapOff
	heapOff = end
	return uint32(uintptr(unsafe.Pointer(&heap[0]))) + off
}

// inputBegin 在每个 handle_* 入口调用：复位输入缓冲，并报告上一分配阶段是否溢出。
// 溢出 → true：输入字节已被污染，调用方必须拒绝本次调用。
func inputBegin() bool {
	overflow := heapOverflow
	heapOverflow = false
	heapOff = 0
	return overflow
}

// inputTooLarge 是溢出时 handle_* 的统一拒绝响应。
func inputTooLarge() uint32 {
	SetResponse(413, []byte(`{"error":"pdk: 请求输入超过静态缓冲 8MB 上限"}`))
	return 3
}

// readBytes 按 绝对地址 + 长度 取一片内存视图（不拷贝）。
func readBytes(ptr, length uint32) []byte {
	if length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}

// ptrLen 返回字节切片底层的 (绝对地址, 长度)，供传给 host 函数。
func ptrLen(b []byte) (uint32, uint32) {
	if len(b) == 0 {
		return 0, 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0]))), uint32(len(b))
}

// ---- 业务类型 ----

// HTTPResponse 是一次（非流式）HTTP 调用的结果。
type HTTPResponse struct {
	Status int
	Body   []byte
}

// Stream 是流式 HTTP 调用的句柄。
type Stream struct {
	handle uint32
}

// ---- 回调注册 ----

var chatHandler func(req, combos []byte) error

// RegisterChat 注册 chat 处理回调。在 main() 中调用。
//   - req：OpenAI ChatRequest 的 JSON 字节（透传，插件自行反序列化）。
//   - combos：KeyModel[] 的 JSON 字节，host 已按轮询/failover 排好序。
// 回调内用 Emit（流式）或 SetResponse（非流式）回推结果，用 ReportUsage 回告配额。
func RegisterChat(fn func(req, combos []byte) error) { chatHandler = fn }

//export handle_chat
func handleChat(reqPtr, reqLen, combosPtr, combosLen uint32) uint32 {
	if inputBegin() { // 上一分配阶段溢出 → 输入已污染，拒绝本次调用
		return inputTooLarge()
	}
	if chatHandler == nil {
		SetResponse(500, []byte("pdk: chat handler 未注册（main 未执行？）"))
		return 3
	}
	req := readBytes(reqPtr, reqLen)
	combos := readBytes(combosPtr, combosLen)
	if err := chatHandler(req, combos); err != nil {
		SetResponse(500, []byte(err.Error()))
		return 1
	}
	return 0
}

// ---- 其它端点导出（embed/image/speech）----
//
// 这些端点都是【非流式】：插件用 SetResponse(200, bodyJSON) 回推（speech 的 body 是音频字节），
// 不用 Emit。host 的 Instance.CallEndpoint 按端点名分发到对应 handle_<name>。
// 一个插件按需实现其中任意几个——不实现的就不导出，host 探测后该端点对该插件不可用。
//
// 与 handle_chat 同形签名：(reqPtr, reqLen, combosPtr, combosLen) → code。
//   - req：对应 OpenAI 请求体的 JSON 字节（embeddings/images/audio-speech 各自的形状，透传）。
//   - combos：KeyModel[] JSON（host 按轮询/failover 排好序，用法同 chat）。

var embedHandler func(req, combos []byte) error

// RegisterEmbed 注册 /v1/embeddings 处理回调。req 是 EmbedRequest JSON；SetResponse 回推 EmbedResponse JSON。
func RegisterEmbed(fn func(req, combos []byte) error) { embedHandler = fn }

//export handle_embed
func handleEmbed(reqPtr, reqLen, combosPtr, combosLen uint32) uint32 {
	if inputBegin() { // 上一分配阶段溢出 → 输入已污染，拒绝本次调用
		return inputTooLarge()
	}
	if embedHandler == nil {
		SetResponse(500, []byte("pdk: embed handler 未注册"))
		return 3
	}
	req := readBytes(reqPtr, reqLen)
	combos := readBytes(combosPtr, combosLen)
	if err := embedHandler(req, combos); err != nil {
		SetResponse(500, []byte(err.Error()))
		return 1
	}
	return 0
}

var imageHandler func(req, combos []byte) error

// RegisterImage 注册 /v1/images/generations 处理回调。req 是 ImageRequest JSON；SetResponse 回推 ImageResponse JSON（含 b64 或 url）。
func RegisterImage(fn func(req, combos []byte) error) { imageHandler = fn }

//export handle_image
func handleImage(reqPtr, reqLen, combosPtr, combosLen uint32) uint32 {
	if inputBegin() { // 上一分配阶段溢出 → 输入已污染，拒绝本次调用
		return inputTooLarge()
	}
	if imageHandler == nil {
		SetResponse(500, []byte("pdk: image handler 未注册"))
		return 3
	}
	req := readBytes(reqPtr, reqLen)
	combos := readBytes(combosPtr, combosLen)
	if err := imageHandler(req, combos); err != nil {
		SetResponse(500, []byte(err.Error()))
		return 1
	}
	return 0
}

var speechHandler func(req, combos []byte) error

// RegisterSpeech 注册 /v1/audio/speech（TTS）处理回调。req 是 SpeechRequest JSON；
// SetResponse(200, audioBytes) 回推音频字节（host 会按 audio/mpeg 写出，插件内部可按实际格式定）。
func RegisterSpeech(fn func(req, combos []byte) error) { speechHandler = fn }

//export handle_speech
func handleSpeech(reqPtr, reqLen, combosPtr, combosLen uint32) uint32 {
	if inputBegin() { // 上一分配阶段溢出 → 输入已污染，拒绝本次调用
		return inputTooLarge()
	}
	if speechHandler == nil {
		SetResponse(500, []byte("pdk: speech handler 未注册"))
		return 3
	}
	req := readBytes(reqPtr, reqLen)
	combos := readBytes(combosPtr, combosLen)
	if err := speechHandler(req, combos); err != nil {
		SetResponse(500, []byte(err.Error()))
		return 1
	}
	return 0
}

// videoHandler 是 handle_video 的回调（/v1/videos，OpenAI Videos API：异步任务制）。
var videoHandler func(req, combos []byte) error

// RegisterVideo 注册 /v1/videos 处理回调。与 chat 同形签名，但 req 有三种形态
// （host 按请求方法构造，插件按 "_op" 分发；这是 host↔插件的 ABI 约定，对外仍是
// 纯 OpenAI Videos 形状）：
//   - 创建（POST /v1/videos）：req = 原始请求体（含 model），无 _op 键；
//     插件转发 POST {base}/videos（model 替换），SetResponse 回传上游任务 JSON。
//     host 在成功响应里解析 "id" 登记任务路由（见 gateway.go video_tasks）。
//   - 查询（GET /v1/videos/{id}）：req = {"_op":"get","video_id":...}；
//     插件转发 GET {base}/videos/{id}。
//   - 取消（DELETE /v1/videos/{id}）：req = {"_op":"cancel","video_id":...}；
//     插件转发 DELETE {base}/videos/{id}。
//
// 查询/取消的 combos 用【创建时登记的那个账号】（host 从 video_tasks 取快照注入），
// 保证鉴权上下文与创建一致。
func RegisterVideo(fn func(req, combos []byte) error) { videoHandler = fn }

//export handle_video
func handleVideo(reqPtr, reqLen, combosPtr, combosLen uint32) uint32 {
	if inputBegin() { // 上一分配阶段溢出 → 输入已污染，拒绝本次调用
		return inputTooLarge()
	}
	if videoHandler == nil {
		SetResponse(500, []byte("pdk: video handler 未注册"))
		return 3
	}
	req := readBytes(reqPtr, reqLen)
	combos := readBytes(combosPtr, combosLen)
	if err := videoHandler(req, combos); err != nil {
		SetResponse(500, []byte(err.Error()))
		return 1
	}
	return 0
}

// actionHandler 是 handle_action 的回调。action 名与 payload JSON 由插件自定义解释，
// host 完全不解读（框架隔离约定）。结果经 SetResponse(200, resultJSON) 回传；
// 应用层错误用 SetResponse(4xx/5xx, errJSON)；返回 go error 仅用于内部异常（→ return 1）。
var actionHandler func(action string, payload []byte) error

// HandleAction 注册 action 处理回调（通用插件 RPC 入口）。在 main() 中调用。
// UI / 定时器 / 网关统一经 host 的 Instance.Action → handle_action 调到这里。
// action 名（如 "login_send_code"、"refresh"）由各插件自行定义，主包不认。
func HandleAction(fn func(action string, payload []byte) error) { actionHandler = fn }

//export handle_action
func handleAction(actionPtr, actionLen, payloadPtr, payloadLen uint32) uint32 {
	if inputBegin() { // 上一分配阶段溢出 → 输入已污染，拒绝本次调用
		return inputTooLarge()
	}
	if actionHandler == nil {
		// 不支持 action 的插件（仅注册了 chat）：告知 host 此导出虽在但无处理器。
		SetResponse(501, []byte(`{"error":"pdk: action handler 未注册"}`))
		return 3
	}
	action := string(readBytes(actionPtr, actionLen))
	payload := readBytes(payloadPtr, payloadLen)
	if err := actionHandler(action, payload); err != nil {
		SetResponse(500, []byte(err.Error()))
		return 1
	}
	return 0
}

// ---- 输出与配置 API ----

// host 侧 chat 通用整形（manifest `chat_request_patch` 声明，convo ≥ 2026-08-20）完成
// 后注入调用的保留配置键。插件读 ChatPatchedKey 非空 = 请求体已在 host Go 侧整形、
// 原样透传即可（wasm 内零 JSON 工作）；ForceStream 整形后 body 的 stream 恒为 true，
// 客户端真实流式意图改读 ClientStreamKey（"true"/"false"）。旧 host 不注入这两个键
// ——市场分发的插件须保留本地整形与 body stream 解析作回退（pdk.ChatPatchedKey
// 探测切换）。与 host 侧 chatpatch.go 的常量字节级一致。
const (
	ChatPatchedKey  = "__convo_chat_patched"
	ClientStreamKey = "__convo_client_stream"
)

// Log 写一条日志。level: 0=debug 1=info 2=warn 3=error。
func Log(level int, msg string) {
	p, l := ptrLen([]byte(msg))
	hostLog(uint32(level), p, l)
}

// SetResponse 设置非流式响应（状态码 + 响应体）。
func SetResponse(status int, body []byte) {
	p, l := ptrLen(body)
	hostSetResponse(uint32(status), p, l)
}

// Emit 写一段流式输出（OpenAI-SSE 字节，可多次调用）。
func Emit(b []byte) {
	p, l := ptrLen(b)
	hostEmit(p, l)
}

// ReportUsage 回告实际使用的 model/key（供配额统计）。
func ReportUsage(model, key string) {
	mp, ml := ptrLen([]byte(model))
	kp, kl := ptrLen([]byte(key))
	hostReportUsage(mp, ml, kp, kl)
}

// getConfigBuf 是 GetConfig 的读出缓冲（包级静态：64KB 不占栈；wasm 单线程无并发问题）。
var getConfigBuf [64 << 10]byte

// GetConfig 读取插件实例配置项（host 在实例化时注入，如 base_url）。
// key 以 "*" 结尾时做前缀枚举：返回命中键值对的 JSON 对象文本（如 "st:mx:*" →
// {"st:mx:free:glm-5.3:1":"ok",...}），供插件枚举自管状态键。
// 值超 64KB 时 host 返回 0xffffffff，此处记日志并回 ""（配置项实际远小于此上限）。
func GetConfig(key string) string {
	kp, kl := ptrLen([]byte(key))
	outPtr := uint32(uintptr(unsafe.Pointer(&getConfigBuf[0])))
	n := hostGetConfig(kp, kl, outPtr, uint32(len(getConfigBuf)))
	if n == 0xffffffff {
		Log(2, "pdk: GetConfig 值超过 64KB 读出缓冲（返回空）")
		return ""
	}
	return string(getConfigBuf[:n])
}

// SetKV 写插件运行时状态（host 落 plugin_state 表，key/value 原样透传不解读）。
// 同一调用内后续 GetConfig 立即可见；跨调用经 host 注入生效（设置快照 TTL 最迟
// 十几秒，写后 host 会主动失效缓存）。键建议带插件自用前缀（如 "st:"）避免与
// 用户设置键混淆。键 ≤128B、值 ≤64KB，超限静默丢弃并记日志。
func SetKV(key, value string) {
	kp, kl := ptrLen([]byte(key))
	vp, vl := ptrLen([]byte(value))
	hostSetKV(kp, kl, vp, vl)
}

// ---- HTTP API（命令式核心）----

// lastErrorText 读 host 侧最近一次调用失败的真因（last_error 导出对；无记录
// 返回空串）。失败导出只回 0/-1，真因从这里补——调用方错误路径立即读，guest
// 单线程保证时序。
func lastErrorText() string {
	n := hostLastErrorLen()
	if n == 0 {
		return ""
	}
	out := make([]byte, n)
	outPtr := uint32(uintptr(unsafe.Pointer(&out[0])))
	got := hostLastError(outPtr, n)
	if got == 0xffffffff {
		return ""
	}
	return string(out[:got])
}

// failErr 构造带真因的错误：cause 为空时退回通用文案（host 过老等防御）。
func failErr(generic string) error {
	if c := lastErrorText(); c != "" {
		return errors.New(generic + ": " + c)
	}
	return errors.New(generic)
}

// IsCanceled 判断错误是否为调用方取消（客户端断开）。真因经 last_error 以字符串
// 传来，无法 errors.Is——host 的流跟随请求 ctx，取消即客户端断开，特征子串稳定。
// 插件据此在取消时静默收尾（不 SetResponse 错误体——客户端已走，写了也是废字节，
// 且网关会把 502 误记为 ERR）。
func IsCanceled(err error) bool {
	return err != nil && strings.Contains(err.Error(), "context canceled")
}

// HTTPRequest 发起一次非流式 HTTP 请求。headers 为 JSON 序列化后传给 host。
func HTTPRequest(method, url string, headers map[string]string, body []byte) (*HTTPResponse, error) {
	hdr, err := json.Marshal(headers)
	if err != nil {
		return nil, err
	}
	mp, ml := ptrLen([]byte(method))
	up, ul := ptrLen([]byte(url))
	hp, hl := ptrLen(hdr)
	bp, bl := ptrLen(body)
	h := hostHTTPRequest(mp, ml, up, ul, hp, hl, bp, bl)
	if h == 0 {
		return nil, failErr("http_request 失败")
	}
	defer hostResponseClose(h)
	status := int(hostResponseStatus(h))
	bodyLen := hostResponseBodyLen(h)
	respBody := make([]byte, bodyLen)
	if bodyLen > 0 {
		outPtr := uint32(uintptr(unsafe.Pointer(&respBody[0])))
		n := hostResponseBody(h, outPtr, bodyLen)
		if n == 0xffffffff {
			return nil, errors.New("读取响应体失败")
		}
		respBody = respBody[:n]
	}
	return &HTTPResponse{Status: status, Body: respBody}, nil
}

// HTTPStreamOpen 发起一次流式 HTTP 请求，返回可逐块读取的 Stream。
func HTTPStreamOpen(method, url string, headers map[string]string, body []byte) (*Stream, error) {
	hdr, err := json.Marshal(headers)
	if err != nil {
		return nil, err
	}
	mp, ml := ptrLen([]byte(method))
	up, ul := ptrLen([]byte(url))
	hp, hl := ptrLen(hdr)
	bp, bl := ptrLen(body)
	h := hostHTTPStreamOpen(mp, ml, up, ul, hp, hl, bp, bl)
	if h == 0 {
		return nil, failErr("http_stream_open 失败")
	}
	return &Stream{handle: h}, nil
}

// Next 读取下一段流字节。eof 为 true 表示上游已结束。
func (s *Stream) Next() (chunk []byte, eof bool, err error) {
	var buf [16384]byte
	outPtr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	n := hostStreamNext(s.handle, outPtr, uint32(len(buf)))
	switch {
	case n == 0xffffffff:
		return nil, false, failErr("stream 读取错误")
	case n == 0:
		return nil, true, nil // EOF
	}
	// buf 在下次 Next 会复用，必须拷出。
	return append([]byte(nil), buf[:n]...), false, nil
}

// Close 关闭流。
func (s *Stream) Close() { hostStreamClose(s.handle) }

// Status 返回上游 HTTP 状态码（stream handle 与 response handle 共用句柄表，
// 故直接复用 response_status）。用于流式上游的 401/5xx 检测：在首次 Next 前 peek，
// 非 2xx 时别把错误体当 SSE emit，改走 SetResponse 干净报错。
func (s *Stream) Status() int { return int(hostResponseStatus(s.handle)) }

// ---- 响应句柄读取（低阶）----
// 下列函数直接操作响应句柄（来自 HTTPRequest 内部已自动关闭，故仅对 SessionRequest 返回的
// 句柄有意义）。SessionRequest 不自动关闭——插件按需读 status/body/header 后须 ResponseClose。

// ResponseStatus 读响应状态码。
func ResponseStatus(handle uint32) int { return int(hostResponseStatus(handle)) }

// ResponseBody 读响应体（整块拷出）。
func ResponseBody(handle uint32) ([]byte, error) {
	n := hostResponseBodyLen(handle)
	if n == 0xffffffff {
		return nil, errors.New("读取响应体长度失败")
	}
	if n == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	outPtr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	got := hostResponseBody(handle, outPtr, n)
	if got == 0xffffffff {
		return nil, errors.New("读取响应体失败")
	}
	return buf[:got], nil
}

// ResponseHeader 读指定响应头（大小写不敏感）；不存在返回 ""。
func ResponseHeader(handle uint32, name string) string {
	var buf [8192]byte
	np, nl := ptrLen([]byte(name))
	outPtr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	n := hostResponseHeader(handle, np, nl, outPtr, uint32(len(buf)))
	if n == 0xffffffff {
		return ""
	}
	return string(buf[:n])
}

// ResponseClose 释放响应句柄。
func ResponseClose(handle uint32) { hostResponseClose(handle) }

// ---- HTTP 会话（cookie jar + 可控重定向）----
// 用于多步登录 / 续期编排：一个 session 跨多次 SessionRequest 保持 cookie。

// SessionOpen 创建一个有状态会话，返回句柄（>0）。
func SessionOpen() (uint32, error) {
	h := hostSessionOpen()
	if h == 0 {
		return 0, errors.New("session_open 失败")
	}
	return h, nil
}

// SessionRequest 用会话发一次请求，返回响应句柄（0=失败，调用方判 err）。
// 跨多次调用保持 cookie；follow=false 时不跟随重定向（返回 3xx 本身，供读 Location）。
// 返回的句柄须由调用方在读完后 ResponseClose 释放。
func SessionRequest(session uint32, method, url string, headers map[string]string, body []byte, follow bool) (uint32, error) {
	hdr, err := json.Marshal(headers)
	if err != nil {
		return 0, err
	}
	mp, ml := ptrLen([]byte(method))
	up, ul := ptrLen([]byte(url))
	hp, hl := ptrLen(hdr)
	bp, bl := ptrLen(body)
	var f uint32
	if follow {
		f = 1
	}
	h := hostSessionRequest(session, mp, ml, up, ul, hp, hl, bp, bl, f)
	if h == 0 {
		return 0, errors.New("session_request 失败")
	}
	return h, nil
}

// SessionClose 释放整个会话（其 cookie jar 一并丢弃）。
func SessionClose(session uint32) { hostSessionClose(session) }

// SessionCookies 把会话 jar 里属于 url 的 cookie 序列化成 "k=v; k=v2"。
// 登录完成后调用，把 cookie 存进 credentials，供后续刷新时预置回新会话。
func SessionCookies(session uint32, url string) string {
	var buf [16384]byte
	up, ul := ptrLen([]byte(url))
	outPtr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	n := hostSessionCookies(session, up, ul, outPtr, uint32(len(buf)))
	if n == 0xffffffff {
		return ""
	}
	return string(buf[:n])
}

// SessionSetCookies 把保存的 cookie 字符串（"k=v; k=v2"）预置回会话 jar，
// 让后续请求自动带这些 cookie。刷新流程开始时调用：把账号 cookies 注入 session，
// 这样 [2] 绑定步骤会自动带 keycloak 登录态 cookie，不再需要手动设 Cookie header。
// 返回写入的 cookie 数（0=失败/无）。
func SessionSetCookies(session uint32, url, cookies string) uint32 {
	up, ul := ptrLen([]byte(url))
	cp, cl := ptrLen([]byte(cookies))
	return hostSessionSetCookies(session, up, ul, cp, cl)
}
