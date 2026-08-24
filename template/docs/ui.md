# 插件 UI 规范

插件 UI = 单 HTML 文件（`src/ui/index.html`，TinyGo `//go:embed` 内嵌），出现在
convo 控制台·平台管理页的 iframe 里。**无构建、无框架、无外部依赖**——纯 HTML/CSS/JS。

模板 `src/ui/index.html` 是权威样板（设计 token + 组件 + 账号管理全流程）。做新 UI
从它出发改，不要从零写。

## 设计 token（与一方插件完全同源）

token 值逐项取自 convo 主应用 `fnos-theme.css` 的明暗两套 Semi 变量（fnOS Semi Design
风格），与 platform / workbuddy / zcode 三个一方插件的 UI **完全一致**——生态插件与
内置插件观感统一，用户无割裂感。

```css
:root { /* 亮色 */ --bg-1 --bg-2 --bg-dropdown --text-0..3 --primary(-hover/-active)
        --success --warning --danger (+ 各 -light) --fill-0/1/2
        --border(-hover) --focus --divider --overlay --shadow-elevated }
.dark { /* 暗色：同名变量换值 */ }
```

铁律：

1. **所有颜色一律走变量**，禁止硬编码色值（暗色会破）。
2. 暗色切换 = `html.dark` class（宿主主题脚本驱动：初始 `#dark` 片段 +
   `convo:theme` postMessage），token 自动换色。
3. 页面底色**显式 `var(--bg-1)`**，不能用透明——`color-scheme:dark` 时 Chrome 会把
   透明画布涂黑（workbuddy 实案）。

## 组件（形态对齐一方插件：Card 12px 圆角 + divider 描边；按钮 8px 圆角/字重 500；输入框 6px + 边框三态）

| class | 用途 |
|---|---|
| `.card` | 页面容器（居中 620px 列布局） |
| `.btn` / `.btn.primary` / `.btn.danger` | 默认 / 主操作 / 危险按钮 |
| `.input` | 文本输入（hover/focus 边框三态） |
| `.table` | 数据表（th 灰、divider 行分隔） |
| `.tag` / `.tag.ok` / `.tag.warn` | 状态标签（-light 底 + 实色字） |
| `.row` / `.muted` / `hr` | 横排 / 次要文字 / 分隔线 |

## 硬性要求（详见 rules.md / ui 样板注释）

1. fetch 一律 `ABS + path`，ABS 采信校验写法照抄模板（同源前缀校验）。
2. 主题跟随 + 环境自检横幅两段脚本**保留勿删**（对应真实事故）。
3. 账号管理走标准契约：`verify_key` action 出落库三件套 → `POST /accounts` 落库；
   列表 `GET /accounts`（`display` 是插件自定义 JSON，UI 自行解析展示）；
   删除 `DELETE /accounts/{id}`。

## 布局尺度

- 字号：正文 14px / 次要 13px / 标题 16px（字重 600）；行高 1.65。
- 间距：容器 gap 14px、页边距 20px；按钮 6×14 内边距；圆角按组件（8/6/12）。
- 页宽：`.card` max-width 620px 居中（管理型页面，非全屏应用）。
