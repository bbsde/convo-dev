# AGENTS.md — __PLUGIN_NAME__（convo 插件）

本目录是一个 [Convo](https://github.com/bbsde/convo) 插件项目（TinyGo 编译 → wasm）。
本文件是 AI agent 工作指引：在此开发插件前先读完再动手；人类开发者同样适用。

## 布局

- `src/main.go` — 插件实现：`verify_key`（凭据校验）+ `handle_chat`（流式/非流式中继）+ `echo`
- `src/plugin.json` — manifest：模型清单 / 表声明 / 端点开关（字段参考 `docs/manifest.md`）
- `src/migrations/` — SQLite 迁移（通用账号表标准列）
- `src/ui/` — 插件管理 UI（单 HTML，出现在 convo 控制台里）
- `docs/` — 契约文档（见下）
- `build.sh` — go mod tidy + TinyGo 编译（**wasm 落 `src/`**，与 plugin.json 同目录
  ——host 按目录发现插件要求二者同在，`src/` 即完整可加载插件目录）+ `.cpk` 打包 → `dist/`

## 开发前必读（按序）

1. `docs/rules.md` — 硬性约定与坑，**每条对应真实事故**（wasm 运行期崩溃 / 线性内存打穿 / 内置浏览器 404）
2. `docs/pdk-api.md` — SDK API（回调注册 / HTTP / 会话 / 配置与状态）
3. `docs/manifest.md` — plugin.json 字段参考
4. `docs/testing.md` — 编译、安装调试、打包分发、发布自查

## 典型任务

- **接入某平台**：改 `src/plugin.json` 模型清单（models[].id）→ 按平台协议重写
  `src/main.go` 的 verifyKey / handleChat。平台逆向所得（端点指纹、模型清单、风控）
  记到 `docs/NOTES.md`，边逆向边记。
- **加管理 UI 功能** → `src/ui/`
- **编译验证** → `./build.sh`（wasm → `src/`，`.cpk` → `dist/`）

## 铁律（违反 = 运行期崩溃或验收不过）

1. **echo action 永远保留**且 200 原样回传（发布冒烟关卡）。
2. **wasm 内禁一切 sync 原语**（`sync.Once` / `sync.Mutex` / `sync.WaitGroup`——初始化期
   和运行期调用都会崩）。需要"只做一次"用包级变量判空懒初始化；并发由 host 兜底。
3. **不整份 JSON 树化请求体**：禁 `map[string]any` 整体 decode→Marshal 组合
   （会打穿 wasm 线性内存）；透传场景用浅解码逐字段处理。
4. **插件 UI 的 fetch 一律绝对 URL，且注入值要做同源采信校验**：
   `var INJ = window.__CONVO_ABS__; var ABS = (INJ && INJ.indexOf(location.origin) === 0) ? INJ : (location.origin + (location.pathname.split("/plugins/")[0] || ""))`，
   然后 `fetch(ABS + path)`。只采信与页面同源前缀的注入值（host 注入被污染时自保，
   platform 插件实案）；根相对路径在内置浏览器下会被重定基，必坏。
   保留模板的**环境自检**（`__CONVO_ABS__` 缺失时亮横幅）与**主题跟随**
   （`#dark` 初始片段 + `convo:theme` postMessage）——它们对应真实事故。
5. 提交前自查：`grep -rn "sync\." src/` 应为空；echo 实调 200。

## 调试回路（主路径：.cpk 安装）

`./build.sh` → `dist/*.cpk` → convo 控制台·**市场页拖入 .cpk** 安装 → 平台管理页
打开插件 UI；echo 自检（`docs/testing.md` 有现成 curl 命令）。迭代 = 重新 build 后
再拖入覆盖安装。备选目录方式（免打包直调）：`src/` 软链到 convo 的
`plugins/providers/__PLUGIN_NAME__/` 后重启（build 已把 wasm 编到 src/）。

## 分发

自由插件：`dist/*.cpk` 直接分发。付费插件：上传 Convo 插件市场定价销售
（license 由市场在线签发，开发者不接触签名）。详见 `docs/testing.md`。
