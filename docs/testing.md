# 编译、调试与分发

## 前置工具

| 工具 | 用途 | 安装 |
|---|---|---|
| Go ≥ 1.22 | go mod / 依赖管理 | https://go.dev/dl/ |
| TinyGo | 编译 wasm | https://tinygo.org/getting-started/install/ |
| convo 网关 | 运行/调试插件 | https://github.com/bbsde/convo/releases |

## 编译

```bash
cd my-plugin
go mod tidy                                            # 拉取 github.com/bbsde/convo-dev/pdk
tinygo build -target wasi -no-debug -o my-plugin.wasm .
```

产物 = `my-plugin.wasm`；与 `plugin.json` 同目录即构成可安装插件。

> TinyGo 报 `could not find wasm-opt` 时：安装 [binaryen](https://github.com/WebAssembly/binaryen/releases)
> 并设 `WASMOPT=/path/to/wasm-opt(.exe)`（部分 TinyGo 发行版不带它）。

> 迭代 pdk 本身时可用本地替换：`go mod edit -replace github.com/bbsde/convo-dev/pdk=../convo-dev/pdk`

## 安装到 convo 调试

convo 按目录发现插件（`plugin.json` + wasm 同目录）：

```
<plugins-dir>/providers/my-plugin/
  ├── plugin.json
  └── my-plugin.wasm
```

`plugins-dir` 解析顺序：`CONVO_PLUGINS_DIR` env > 二进制同级 `plugins/` > 二进制同级 `../plugins/`（发布布局）。放好后重启 convo（或经市场重登触发重载）。

自检 echo 接线（冒烟同款）：

```bash
curl -X POST http://127.0.0.1:8080/api/plugins/my-plugin/action \
  -H 'Content-Type: application/json' \
  -d '{"action":"echo","payload":{"ping":1}}'
# 期望：200，原样回 {"ping":1}
```

UI 调试：控制台 → 平台/插件页 → 打开插件 UI（host 注入 `__CONVO_ABS__`）。

> 被 license 验签 skip 的插件不会被目录轮询自动重试——重装/市场重登或重启 convo。

## 打包分发

```bash
tar -czf my-plugin-0.1.0.cpk plugin.json my-plugin.wasm
```

- **自由插件**：.cpk 直接分发（用户经市场安装或放入插件目录）。
- **付费插件**：上传市场定价销售，license 由市场按订单在线签发（见 rules.md §8）。

## 发布前自查

- [ ] `echo` action 实现且 200 原样回传
- [ ] 无任何 sync 原语（`grep -rn "sync\." .` 应为空）
- [ ] 无整份请求体树化（`map[string]any` + Marshal 组合）
- [ ] UI fetch 全部绝对 URL（`__CONVO_ABS__`）
- [ ] 表名/插件名合规；migration 建表含标准列
- [ ] `plugin.json` 的 models/version/description 与实际一致
