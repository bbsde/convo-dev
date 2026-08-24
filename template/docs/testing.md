# 编译、调试与分发

## 前置工具

```bash
convo-dev setup    # 一键体检 + 自动安装缺失工具（TinyGo / binaryen → ~/.convo-dev/tools）
convo-dev doctor   # 只体检不安装
```

手动安装（可选）：[Go ≥ 1.22](https://go.dev/dl/) · [TinyGo](https://tinygo.org/getting-started/install/) ·
[binaryen（wasm-opt）](https://github.com/WebAssembly/binaryen/releases)（TinyGo 缺 wasm-opt 时设 `WASMOPT=<路径>`）。
运行插件还需要 [convo 网关](https://github.com/bbsde/convo/releases)。

## 编译

```bash
cd my_plugin
go mod tidy                                            # 拉取 github.com/bbsde/convo-dev/pdk
tinygo build -target wasi -no-debug -o my_plugin.wasm .
```

产物 = `my_plugin.wasm`；与 `plugin.json` 同目录即构成可安装插件。

> TinyGo 报 `could not find wasm-opt` 时：安装 [binaryen](https://github.com/WebAssembly/binaryen/releases)
> 并设 `WASMOPT=/path/to/wasm-opt(.exe)`（部分 TinyGo 发行版不带它）。

> 迭代 pdk 本身时可用本地替换：`go mod edit -replace github.com/bbsde/convo-dev/pdk=../convo-dev/pdk`

## 安装到 convo 测试（主路径：.cpk 拖入安装）

`./build.sh` 产出 `dist/my_plugin-<ver>.cpk` → convo 控制台 → **市场页拖入 .cpk**（或文件选择按钮）→ 安装完成出现插件卡片 → 到平台管理页测试（打开插件 UI、发请求）。

**迭代更新**：改代码 → 重新 `./build.sh` → 再拖入新 `.cpk` 覆盖安装（同 `name` 即更新）。

自检 echo 接线（冒烟同款）：

```bash
curl -X POST http://127.0.0.1:8080/api/plugins/my_plugin/action \
  -H "Content-Type: application/json" \
  -d '{"action":"echo","payload":{"ping":1}}'
# 期望：200，原样回 {"ping":1}
```

UI 调试：控制台 → 平台管理页 → 打开插件 UI（host 注入 `__CONVO_ABS__`）。

> 被 license 验签 skip 的插件不会被目录轮询自动重试——重装/市场重登或重启 convo。

## 备选：目录方式（免打包直调）

convo 也按目录发现插件（`plugin.json` + wasm 同目录）：

```
<plugins-dir>/providers/my_plugin/
  ├── plugin.json
  └── my_plugin.wasm
```

wasm 由 `./build.sh` 编译到 `src/`（与 plugin.json 同目录），把 `src/` 软链或复制到上述位置后重启 convo 即可。`plugins-dir` 解析顺序：`CONVO_PLUGINS_DIR` env > 二进制同级 `plugins/` > 二进制同级 `../plugins/`（发布布局）。

## 打包分发

`.cpk` 已由 `./build.sh` 打包到 `dist/`（plugin.json + wasm 的 tar.gz）。

- **自由插件**：`.cpk` 直接分发（用户在 convo 市场页拖入安装）。
- **付费插件**：上传市场定价销售，license 由市场按订单在线签发（见 rules.md §8）。

## 发布前自查

- [ ] `echo` action 实现且 200 原样回传
- [ ] 无任何 sync 原语（`grep -rn "sync\." .` 应为空）
- [ ] 无整份请求体树化（`map[string]any` + Marshal 组合）
- [ ] UI fetch 全部绝对 URL（`__CONVO_ABS__`）
- [ ] 表名/插件名合规；migration 建表含标准列
- [ ] `plugin.json` 的 models/version/description 与实际一致
