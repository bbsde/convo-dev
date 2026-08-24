// convo-dev 是 convo 插件脚手架与环境配置 CLI。
//
// 安装：
//
//	go install github.com/bbsde/convo-dev@latest
//
// 命令：
//
//	convo-dev init <name>              # 生成插件项目（远程拉模板；详见 convo-dev init -h）
//	convo-dev doctor                   # 环境体检：检查 go / tinygo / wasm-opt 并报告
//	convo-dev setup                    # 自动安装缺失工具（TinyGo / binaryen → ~/.convo-dev/tools）
//	convo-dev env                      # 输出 shell 集成行（eval "$(convo-dev env)"）
//	convo-dev version
//
// init 生成的项目结构（模板占位符 __PLUGIN_NAME__ 会被替换为 <name>）：
//
//	<name>/
//	├── build.sh        # tidy + TinyGo 编译 + cpk 打包（产物进 dist/）
//	├── dist/           # 构建产物（<name>.wasm / <name>-<ver>.cpk）
//	├── docs/           # 项目文档（README 骨架 + NOTES 逆向笔记模板）
//	└── src/            # 源码：main.go / plugin.json / go.mod / migrations/ / ui/
package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
)

const (
	repoName    = "convo-dev"
	downloadFmt = "https://codeload.github.com/bbsde/%s/tar.gz/refs/%s/%s"
	nameRe      = `^[a-z][a-z0-9_]{1,30}$`
	placeholder = "__PLUGIN_NAME__"

	// setup 默认安装版本（与 convo 一方插件实际构建环境一致；可用 --tinygo/--binaryen 覆盖）
	defaultTinyGoVer   = "0.41.1"
	defaultBinaryenVer = "120"
)

var validName = regexp.MustCompile(nameRe)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "init":
		runInit(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "setup":
		runSetup(os.Args[2:])
	case "env":
		runEnv(os.Args[2:])
	case "version", "-v", "--version":
		printVersion()
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "✗ 未知命令：%s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`convo-dev — convo 插件脚手架与环境配置

用法：
  convo-dev init <name> [选项]   生成插件项目（name 规则 ^[a-z][a-z0-9_]{1,30}$）

init 选项：
  -d <dir>        目标目录（默认 ./<name>）
  --ref <ref>     模板分支/tag（默认 main）
  --url <url>     完整 tarball 地址（镜像/离线包，覆盖默认 codeload）
  --template <d>  本地模板目录（含 src/ docs/ build.sh，跳过远程拉取）

  convo-dev doctor   环境体检（go / tinygo / wasm-opt 检测与版本）
  convo-dev setup    自动安装缺失工具到 ~/.convo-dev/tools
                    选项：--tinygo <ver> --binaryen <ver> --skip-tinygo --skip-binaryen
                          --tinygo-url/--binaryen-url <完整地址> --mirror <前缀>（或 env CONVO_DEV_MIRROR）
  convo-dev env      输出 shell 集成行：eval "$(convo-dev env)"
  convo-dev version
`)
}

func printVersion() {
	v := "(devel)"
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		v = bi.Main.Version
	}
	fmt.Printf("convo-dev %s (%s/%s)\n", v, runtime.GOOS, runtime.GOARCH)
}
