#!/usr/bin/env bash
# convo 插件脚手架：从 template/ 生成一个新插件项目。
#
# 用法:
#   ./new-plugin.sh <name> [目标目录]     # 目录缺省 = ./<name>
#
# name 规则：^[a-z][a-z0-9_]{1,30}$（同时用作 plugin.json 的 name、wasm 文件名、账号表名前缀）
#
# 生成后:
#   cd <dir>
#   go mod tidy
#   tinygo build -target wasi -no-debug -o <name>.wasm .
# 详细开发/调试/分发指引见 docs/（testing.md / rules.md / manifest.md）。
set -euo pipefail

die() { echo "✗ $*" >&2; exit 1; }

NAME="${1:-}"
DEST="${2:-$NAME}"
[[ "$NAME" =~ ^[a-z][a-z0-9_]{1,30}$ ]] || die "用法: ./new-plugin.sh <name> [目标目录]；name 须匹配 ^[a-z][a-z0-9_]{1,30}$"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$ROOT/template"
[ -d "$SRC" ] || die "模板目录不存在：$SRC（应在 convo-dev 仓库根运行）"
[ -e "$DEST" ] && die "目标已存在：$DEST"

echo "==> 生成插件 $NAME → $DEST"
mkdir -p "$DEST"
cp -r "$SRC/." "$DEST/"

# 占位符替换（go / json / sql / html / go.mod）
find "$DEST" -type f \( -name "*.go" -o -name "*.json" -o -name "*.sql" \
  -o -name "*.html" -o -name "go.mod" \) -print0 |
  xargs -0 sed -i "s/__PLUGIN_NAME__/$NAME/g"

cat <<EOF
✅ 插件已生成：$DEST

下一步：
  1. 编辑 plugin.json：真实模型清单（models[].id）、标题、描述
  2. 按目标平台协议改写 main.go 的 verifyKey / handleChat
  3. 编译（需 TinyGo）：
     cd $DEST && go mod tidy && tinygo build -target wasi -no-debug -o $NAME.wasm .
  4. 安装到 convo 网关调试 / 打包分发：见 docs/testing.md

硬性约定速查（务必遵守，详见 docs/rules.md）：
  - echo action 必须保留（发布冒烟测试依赖）
  - wasm 内禁用一切 sync 原语（sync.Once/Mutex/WaitGroup 会崩）
  - 不做整份请求体 JSON 树化；chat 透传用 chat_request_patch + 浅解码回退
  - 插件 UI 的 fetch 一律绝对 URL（window.__CONVO_ABS__）
EOF
