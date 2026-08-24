// __PLUGIN_NAME__ 插件（TinyGo）：OpenAI 兼容平台接入模板。
//
// 形态：API key 直配（api_key + base_url），对话透传 {base_url}/chat/completions。
// 由 convo-dev 脚手架生成——开发顺序建议：
//   1. 改 plugin.json：真实模型清单（models[].id）、标题、描述
//   2. 按目标平台协议改写 verifyKey / handleChat（身份指纹、协议转换、自定义 action 等）
//   3. ui/index.html 按需扩展（已带 添加账号→verify_key→落库 全流程）
//
// 硬性约定见 docs/rules.md：echo action 必须保留；wasm 内禁用一切 sync
// 原语；不做整份请求体 JSON 树化（浅解码/透传）；插件 UI 一律绝对 URL。

package main

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	pdk "github.com/bbsde/convo-dev/pdk"
)

//go:embed ui/index.html
var uiHTML []byte

//go:embed migrations/1.sql
var migration1 []byte

// credentials 是账号落库形态：verify_key 组装，chat 时从 combos[0].key 还原。
type credentials struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func main() {
	pdk.RegisterChat(handleChat)
	pdk.HandleAction(handleAction)
	pdk.RegisterAsset("ui", uiHTML)
	pdk.RegisterAsset("migration_1", migration1)
}

// failAction 设置应用层错误响应（状态码 + 错误 JSON），handle_action 视为正常返回。
func failAction(code int, msg string) error {
	b, _ := json.Marshal(map[string]string{"error": msg})
	pdk.SetResponse(code, b)
	return nil
}

// handleAction 是通用插件 RPC 入口：UI 与账号级调用都到这里，host 不解读语义。
func handleAction(action string, payload []byte) error {
	switch action {
	case "echo":
		// 标准约定 action（所有插件必须实现）：原样回传 payload——发布冒烟测试依赖它。
		pdk.SetResponse(200, payload)
		return nil
	case "verify_key":
		return verifyKey(payload)
	default:
		return failAction(404, "unknown action: "+action)
	}
}

// verifyKey 校验 api_key + base_url（GET {base}/models 试通），产出落库三件套
// {credentials, display, external_id}。UI 添加账号传 {api_key, base_url}；
// 账号卡重验时 host 注入 {credentials}。
func verifyKey(payload []byte) error {
	var args struct {
		APIKey      string `json:"api_key"`
		BaseURL     string `json:"base_url"`
		Credentials string `json:"credentials"`
	}
	if err := json.Unmarshal(payload, &args); err != nil {
		return failAction(400, "解析参数失败: "+err.Error())
	}
	if args.Credentials != "" { // 账号级重验：从已存 credentials 补缺
		var c credentials
		if json.Unmarshal([]byte(args.Credentials), &c) == nil {
			if args.APIKey == "" {
				args.APIKey = c.APIKey
			}
			if args.BaseURL == "" {
				args.BaseURL = c.BaseURL
			}
		}
	}
	args.APIKey = strings.TrimSpace(args.APIKey)
	args.BaseURL = strings.TrimRight(strings.TrimSpace(args.BaseURL), "/")
	if args.APIKey == "" || args.BaseURL == "" {
		return failAction(400, "api_key 与 base_url 均必填")
	}

	resp, err := pdk.HTTPRequest("GET", args.BaseURL+"/models",
		map[string]string{"Authorization": "Bearer " + args.APIKey}, nil)
	if err != nil {
		return failAction(502, "上游连接失败: "+err.Error())
	}
	if resp.Status == 401 || resp.Status == 403 {
		return failAction(401, "key 无效（HTTP "+strconv.Itoa(resp.Status)+"）")
	}
	if resp.Status >= 400 {
		return failAction(resp.Status, "上游异常 HTTP "+strconv.Itoa(resp.Status))
	}

	credJSON, _ := json.Marshal(credentials{APIKey: args.APIKey, BaseURL: args.BaseURL})
	dispJSON, _ := json.Marshal(map[string]string{
		"base_url":   args.BaseURL,
		"status":     "ok",
		"checked_at": time.Now().UTC().Format("2006-01-02 15:04:05"),
	})
	out, _ := json.Marshal(struct {
		Credentials string `json:"credentials"`
		Display     string `json:"display"`
		ExternalID  string `json:"external_id"` // 同 key 重复添加走 upsert 合并
	}{string(credJSON), string(dispJSON), args.APIKey})
	pdk.SetResponse(200, out)
	return nil
}

// handleChat 是网关 chat 入口。combos：[{key: 账号credentials JSON, model: 上游模型名}]，
// host 已按轮询/failover 排好序——取 combos[0] 即可，失败回 401/5xx 由 host 记错/熔断/failover。
func handleChat(reqBytes, combosBytes []byte) error {
	var combos []struct {
		Key   string `json:"key"`
		Model string `json:"model"`
	}
	if json.Unmarshal(combosBytes, &combos) != nil || len(combos) == 0 {
		return failAction(400, "无可用账号（combo 为空）")
	}
	var creds credentials
	if json.Unmarshal([]byte(combos[0].Key), &creds) != nil || creds.APIKey == "" || creds.BaseURL == "" {
		return failAction(401, "账号 credentials 缺 api_key/base_url")
	}
	model := combos[0].Model

	var peek struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(reqBytes, &peek)

	// host 声明式整形（plugin.json chat_request_patch）时 body 已含上游 model——原样
	// 透传；否则本地浅解码替换（老 host 回退）。禁止整份 map[string]any 树化（内存红线）。
	body := reqBytes
	if pdk.GetConfig(pdk.ChatPatchedKey) == "" {
		var top map[string]json.RawMessage
		if json.Unmarshal(reqBytes, &top) == nil {
			if b, err := json.Marshal(model); err == nil {
				top["model"] = b
			}
			if peek.Stream {
				top["stream_options"] = json.RawMessage(`{"include_usage":true}`)
			}
			if b, err := json.Marshal(top); err == nil {
				body = b
			}
		}
	}

	hdr := map[string]string{
		"Authorization": "Bearer " + creds.APIKey,
		"Content-Type":  "application/json",
	}
	url := creds.BaseURL + "/chat/completions"

	if peek.Stream {
		st, err := pdk.HTTPStreamOpen("POST", url, hdr, body)
		if err != nil {
			return failAction(502, "上游连接失败: "+err.Error())
		}
		if status := st.Status(); status >= 400 { // 错误体别当 SSE 透传，干净回错误
			errBody, _ := readAllStream(st)
			if len(errBody) > 500 {
				errBody = errBody[:500]
			}
			pdk.SetResponse(status, errBody)
			return nil
		}
		emitPassthrough(st, model)
	} else {
		resp, err := pdk.HTTPRequest("POST", url, hdr, body)
		if err != nil {
			return failAction(502, "上游连接失败: "+err.Error())
		}
		pdk.SetResponse(resp.Status, resp.Body)
		if resp.Status < 400 {
			pdk.ReportUsage(model, "__PLUGIN_NAME__-relay")
		}
	}
	return nil
}

// emitPassthrough 逐 chunk 透传上游 OpenAI SSE。
func emitPassthrough(st *pdk.Stream, model string) {
	defer st.Close()
	for {
		chunk, eof, err := st.Next()
		if err != nil {
			if pdk.IsCanceled(err) {
				// 客户端断开（agent 掐流常态）：静默收尾——往死连接写 502 只会留噪声。
				pdk.Log(0, "chat 流取消（客户端断开）")
				return
			}
			errJSON, _ := json.Marshal(map[string]string{"error": "流读取错误: " + err.Error()})
			pdk.SetResponse(502, errJSON)
			return
		}
		if len(chunk) > 0 {
			pdk.Emit(chunk)
		}
		if eof {
			break
		}
	}
	pdk.ReportUsage(model, "__PLUGIN_NAME__-relay")
}

// readAllStream 读尽一个流（上游错误体兜底读取用）。
func readAllStream(st *pdk.Stream) ([]byte, error) {
	defer st.Close()
	var out []byte
	for {
		chunk, eof, err := st.Next()
		if err != nil {
			return out, err
		}
		out = append(out, chunk...)
		if eof {
			return out, nil
		}
	}
}
