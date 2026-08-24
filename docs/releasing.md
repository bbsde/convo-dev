# 发布与版本管理

convo-dev 仓里有**三个独立演化的版本实体**，各有自己的发布方式：

| 实体 | 版本来源 | tag 格式 | 用户怎么拿到 |
|---|---|---|---|
| CLI（`convo-dev` 命令） | git tag | `vX.Y.Z`（仓根） | `go install github.com/bbsde/convo-dev@latest` |
| pdk SDK（`pdk/` 子模块） | git tag | `pdk/vX.Y.Z`（子目录模块标准前缀） | `go get github.com/bbsde/convo-dev/pdk@latest` |
| 模板 + 文档（`template/` `docs/`） | 跟随分支 | 无 tag | `convo-dev init` 拉取（见下） |

CLI 二进制没有 VERSION 文件、没有硬编码版本号：`convo-dev version` 读 Go
buildinfo（`go install @vX.Y.Z` 装出的自动显示该版本），**不存在忘 bump 的问题**。

## 模板与 CLI 的版本对齐

`init` 的默认模板 ref = **CLI 自身版本 tag**（从 buildinfo 读取，仅认纯 `vX.Y.Z`）：
`go install @v0.1.2` 装的 CLI 永远拉 v0.1.2 时点的模板，模板与 CLI/pdk 的契约不会漂移。

- 本地 `go build`（devel / pseudo-version）→ 回退 `main`（开发便利）。
- 模板热修（不打 CLI 版本就发布模板变更）→ 显式 `convo-dev init <name> --ref main`。

## 发布步骤

### CLI 发版（`vX.Y.Z`）

1. 改动合入 main，CHANGELOG.md 当日节登记提交。
2. `git tag vX.Y.Z && git push origin vX.Y.Z`。
3. 验证：`go install github.com/bbsde/convo-dev@vX.Y.Z && convo-dev version`。
4. 若模板也改了：用新装的 CLI 跑一次 `convo-dev init testproj` 确认拉到的是
   `tags/vX.Y.Z` 且构建链路通，删掉测试目录。

### pdk 发版（`pdk/vX.Y.Z`）

1. 改动合入 main（本地联调可先在消费方 go.mod 临时 replace 到本地 pdk/）。
2. `git tag pdk/vX.Y.Z && git push origin pdk/vX.Y.Z`。
3. **升级检查单**（pdk 的两个仓内消费点）：
   - `template/src/go.mod` 的 require——新项目起点版本；
   - convo-private `src/extensions/go.mod`——一方插件锁定版本（`go get …@vX.Y.Z && go mod tidy` 后重编插件验证）。
4. 验证：`go list -m github.com/bbsde/convo-dev/pdk@latest` 应返回新版本。

## 已知坑

- **sum.golang.org 索引延迟**：新 tag 推送后约几分钟~十几分钟内 `@latest` 拉不到；
  期间用 `GOPROXY=direct GOSUMDB=off go install …@vX.Y.Z` 直连验证。
- pdk 是子目录模块：**必须打 `pdk/` 前缀的 tag**（`pdk/v0.2.0`），裸 `v0.2.0` 会被
  CLI 版本序列抢走、pdk 消费方查不到。
- 模板 `go.mod` 的 pdk 版本升级后，若模板代码用到新 API，记得同步发 CLI tag
  （让 init 默认 ref 指向包含新模板的时点）。
