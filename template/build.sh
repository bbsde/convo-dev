#!/usr/bin/env bash
# __PLUGIN_NAME__ 插件构建脚本：src/ 编译（wasm 落 src/）+ dist/ 打包 .cpk。
#
# 用法: ./build.sh          # go mod tidy + TinyGo 编译 + cpk 打包
# 前置: TinyGo（https://tinygo.org）；TinyGo 缺 wasm-opt 时需 binaryen（设 WASMOPT=<路径>）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SRC="$ROOT/src"
DIST="$ROOT/dist"
NAME="__PLUGIN_NAME__"

# 版本从 src/plugin.json 读（仅展示用）
VER="$(grep -oE '"version"[[:space:]]*:[[:space:]]*"[^"]+"' "$SRC/plugin.json" | head -1 | sed -E 's/.*"([^"]+)"$/\1/')"

# TinyGo / wasm-opt 探测：PATH 优先，其次 convo-dev setup 的安装目录（~/.convo-dev/tools）
DEVTOOLS="$HOME/.convo-dev/tools"
TG="$(command -v tinygo || true)"
[ -z "$TG" ] && for c in "$DEVTOOLS/tinygo/bin/tinygo" "$DEVTOOLS/tinygo/bin/tinygo.exe"; do
  [ -x "$c" ] && TG="$c" && break
done
[ -n "$TG" ] || { echo "✗ 需要 TinyGo：convo-dev setup 自动安装，或 https://tinygo.org 手动" >&2; exit 1; }
export TINYGOROOT="$(cd "$(dirname "$TG")/.." && pwd)"

WOPT="${WASMOPT:-}"
[ -n "$WOPT" ] || WOPT="$(command -v wasm-opt || true)"
[ -z "$WOPT" ] && for c in "$DEVTOOLS/binaryen/bin/wasm-opt" "$DEVTOOLS/binaryen/bin/wasm-opt.exe"; do
  [ -x "$c" ] && WOPT="$c" && break
done
[ -n "$WOPT" ] || echo "  (提示：TinyGo 需要 wasm-opt（binaryen）。缺它时跑 convo-dev setup，或从 https://github.com/WebAssembly/binaryen/releases 装并设 WASMOPT=<路径>)" >&2
export WASMOPT="$WOPT"

echo "==> go mod tidy"
( cd "$SRC" && go mod tidy )

echo "==> TinyGo 编译 → $SRC/$NAME.wasm"
# wasm 落在 src/（与 plugin.json 同目录）：host 按目录发现插件时要求二者同在，
# 这样 src/ 自身就是可直接软链/复制到 plugins/providers/<name>/ 的完整插件目录。
( cd "$SRC" && "$TG" build -target wasi -no-debug -o "$NAME.wasm" . )

echo "==> 打包 cpk → $DIST/$NAME-$VER.cpk"
( cd "$SRC" && tar -czf "$DIST/$NAME-$VER.cpk" plugin.json "$NAME.wasm" )

echo "✅ $SRC/$NAME.wasm（目录加载用）+ $DIST/$NAME-$VER.cpk（分发用）（v$VER）"
echo "   调试：把 src/ 软链/复制到 convo 的 plugins/providers/$NAME/ 后重启——见 docs/testing.md"
