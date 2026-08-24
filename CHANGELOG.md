# convo-dev 更新日志

> 插件开发脚手架 CLI（convo-dev init/setup/doctor/env）+ pdk SDK（module `github.com/bbsde/convo-dev/pdk`）+ 项目模板与契约文档。
> 条目 = 提交（`hash` 可溯源）；按日期倒序。改到本仓就顺手在当日（或新建日期）节下补一行。
> pdk 的更早历史见 convo-private 仓 git 历史（原 `src/pdk/CHANGELOG.md`，2026-08-24 随迁移删除）。

## 2026-08-24
- `07c6279` feat(template): 插件 UI 规范——设计 token（CSS 变量亮/暗两套，html[data-theme] 驱动）+ btn/input/table/tag 组件样式集 + 账号列表展示/删除全流程样板（GET/DELETE /accounts 契约示例）；新增 docs/ui.md（token 语义/组件/布局/硬性要求），必读清单五篇；发布 tag v0.1.13（CLI）
- `cc5c453` feat: convo-dev test——静态检查（plugin.json/表名/sync 扫描/wasm）+ 真实网关冒烟（本地 CONVO_DEV_GATEWAY > 缓存 > latest.json 自动下载 sha256 校验；临时实例 install-cpk 上传 + echo 实调，测完自动清理）；wtproj 全绿、ark 旧布局被准确拦截实测；配套公共仓 Release 资产补 latest.json（convo@456d2d6）；发布 tag v0.1.12（CLI）

- `5a11e08` feat: Windows 系统代理自动探测——Go 不读系统代理只读环境变量（Clash「系统代理」模式实案：PS 用户直连 GitHub 超时）；newHTTPClient 环境变量优先、注册表兜底（ProxyEnable/ProxyServer 含分号格式），update 与 init 模板拉取统一接入；实测无环境变量 shell 下旧版超时、新版秒通；发布 tag v0.1.11（CLI）
- `e72befb` fix: update 抗弱网——tags.atom 超时 15s→30s、失败自动重试一次、报错提示 HTTPS_PROXY 代理设置（Go 读代理环境变量；直连 GitHub 波动实案：本机 Clash shell 通、用户终端直连超时）；发布 tag v0.1.10（CLI）
- `fd1ee6c` feat(template): ABS 注入值同源采信校验成为标准写法——只采信以页面 location.origin 开头的 __CONVO_ABS__，否则回退 pathname 推断（convo ≤1.0.98 host 注入被 Wails 宿主 Referer 污染实案；platform 插件靠此防御自保而模板旧写法中招）；模板 UI/rules.md/AGENTS.md 三处同步；发布 tag v0.1.9（CLI）
- `a9fc2ad` fix(template): 产物位置对齐 host 发现约定——wasm 编译输出 dist/→src/（host 要求 wasm 与 plugin.json 同目录；此前照文档软链 src/ 必报找不到 wasm，ark 插件实案定位）；调试主路径改为 build → dist/*.cpk → 控制台市场页拖入安装（即装即载免重启），目录方式降备选；模板补 .gitignore；实测 install-cpk API 全链路 loaded+echo 200；发布 tag v0.1.8（CLI）
- `cced533` feat(template): UI 样板补全——`__CONVO_ABS__` 环境自检横幅（缺失 = convo 版本过旧/非平台管理页入口，直说原因不静默挂）+ 宿主主题跟随（#dark 初始片段 + convo:theme postMessage + colorScheme 应用）；铁律注明保留防护；发布 tag v0.1.7（CLI）
- `60be5c3` feat: convo-dev update 自更新——tags.atom 查最新 tag（无认证无限流，GitHub API 匿名 403 实测排除）→ 对比 buildinfo 当前版 → GOPROXY=direct 直连 go install（绕 sumdb 新 tag 索引延迟，新 tag 推出立即可更）；发布 tag v0.1.6（CLI）
- `3d109c7` docs: 示例插件名 my-plugin → my_plugin（README + 模板 testing.md 共 13 处）——照旧文档敲命令会被 init 拒名；发布 tag v0.1.5（CLI）
- `f4e40e0` fix: init 名称报错说人话——点破连字符不可用原因（name 派生 SQL 表名 <name>_accounts，host 表名白名单禁 -）与改法（my-plugin → my_plugin）；发布 tag v0.1.4（CLI）
- `6b56145` feat: 契约文档随模板分发——manifest/pdk-api/rules/testing 四篇挪入 template/docs/（init 即得全套，修复模板 AGENTS.md 引用断链）；releasing.md 留仓根；模板内 README/main.go/ui/build.sh 指向改本地；发布 tag v0.1.3（CLI）
- `e6a5f11` feat(template): 生成项目自带 AGENTS.md——init 落位列表 +1（占位符随项目名替换）；布局导览 + 必读文档顺序 + 铁律速查 + 调试回路，开发者 init 后目录即 agent 工作区，口述需求即可按规范开发；发布 tag v0.1.2（CLI）
- `93469e2` feat: init 默认 ref 跟随 CLI 自身版本 tag（buildinfo 仅认纯 vX.Y.Z，pseudo-version/devel 回退 main）——模板与 CLI/pdk 契约不再漂移，热修走 --ref main；新增 docs/releasing.md 发布规范（双 tag 序列 / pdk 升级检查单 / sumdb 延迟坑）；发布 tag v0.1.1（CLI）
- `b8ffb96` fix: init 允许 flag 出现在位置参数之后（flag 包默认停在首个位置参数）；模板 build.sh 改用探测到的 TinyGo 路径编译（PATH 外的 ~/.convo-dev/tools 也能用）——狗粮全链路（go install→setup→init→build.sh→wasm/cpk 产物）实测通过
- `73f4844` feat: convo-dev CLI 化——`init` 远程拉模板（codeload tarball）生成 src/dist/docs 三层布局项目；新增 `setup`（自动安装 TinyGo 0.41.1 + binaryen version_120 到 ~/.convo-dev/tools，--mirror/--url 可覆盖）与 `doctor`/`env`；发布 tag v0.1.0（CLI）
- `67d2059` docs(testing): 补 wasm-opt(binaryen) 环境依赖说明
- `44c601c` feat: 插件开发包首版——pdk SDK（tag pdk/v0.1.0）+ 脚手架模板（OpenAI 兼容接入：echo / verify_key 落库三件套 / chat 流式非流式中继 / UI / migration）+ 契约文档四篇（manifest / pdk-api / rules / testing）
