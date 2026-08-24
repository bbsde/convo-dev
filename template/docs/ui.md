# 插件 UI 规范

插件 UI = 单 HTML 文件（`src/ui/index.html`，TinyGo `//go:embed` 内嵌），出现在
convo 控制台·平台管理页的 iframe 里。**无构建、无框架、无外部依赖**——纯 HTML/CSS/JS。

模板 `src/ui/index.html` 是权威样板：设计 token、组件样式、账号管理全流程（验证添加 /
列表展示 / 删除）都可直接抄。做新 UI 从它出发改，不要从零写。

## 设计 token（`<style>` 顶部，勿改语义）

```css
:root    { --bg --bg2 --text --text2 --primary --primary-h --danger --ok --warn --border --radius }
html[data-theme=dark] { …暗色覆盖… }
```

- 颜色对齐 convo 控制台观感；**所有颜色一律走变量**，禁止硬编码色值（暗色会破）。
- 暗色切换由宿主驱动：模板的主题跟随脚本会把 `html[data-theme]` 置为 `light|dark`
  （初始 `#dark` 片段 + `convo:theme` postMessage），CSS 变量自动换色。

## 组件清单（模板已内置样式，class 即用）

| class | 用途 |
|---|---|
| `.card` | 页面容器（居中 560px 列布局） |
| `.btn` / `.btn.ghost` / `.btn.danger` | 主 / 次要 / 危险按钮 |
| `.input` | 文本输入（聚焦主色描边） |
| `.table` | 数据表（th 灰、行分隔线） |
| `.tag` / `.tag.ok` / `.tag.off` | 状态标签 |
| `.row` | 横排布局（flex wrap） |
| `.muted` | 次要文字 |

## 硬性要求（详见 rules.md，此处速查）

1. fetch 一律 `ABS + path`，ABS 采信校验写法照抄模板（同源前缀校验）。
2. 主题跟随 + 环境自检横幅两段脚本**保留勿删**（对应真实事故）。
3. 账号管理走标准契约：`verify_key` action 出落库三件套 → `POST /accounts` 落库；
   列表 `GET /accounts`（`display` 是插件自定义 JSON，UI 自行解析展示）；
   删除 `DELETE /accounts/{id}`。

## 布局尺度

- 字号：正文 14px / 次要 13px / 标题 18px；行高 1.65。
- 间距：容器 gap 16px；按钮内边距 8×18；圆角统一 `--radius`。
- 页宽：`.card` max-width 560px 居中（管理型页面，非全屏应用）。
