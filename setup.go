// setup.go — convo-dev doctor/setup/env：插件开发环境体检与自动配置。
//
// 工具安装约定：全部落 ~/.convo-dev/tools/<name>/（用户级，不动系统目录、不要管理员权限）：
//
//	~/.convo-dev/tools/tinygo/bin/tinygo(.exe)      TINYGOROOT = ~/.convo-dev/tools/tinygo
//	~/.convo-dev/tools/binaryen/bin/wasm-opt(.exe)
//
// PATH 集成用 `eval "$(convo-dev env)"`；生成的项目 build.sh 也会自动探测该目录。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// tool 是一个可自动安装的开发工具。
type tool struct {
	title   string
	binName string                  // 可执行名（含 .exe 由 binaryPath 补）
	verArgs []string                // 版本子命令
	probe   func() (string, bool)   // 探测（PATH → ~/.convo-dev/tools）
	install func(mirror string) error
}

// toolsRoot 返回 ~/.convo-dev/tools。
func toolsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		fatalf("取用户目录失败：%v", err)
	}
	return filepath.Join(home, ".convo-dev", "tools")
}

// binPath 按平台补 .exe。
func binPath(dir, name string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, name+".exe")
	}
	return filepath.Join(dir, name)
}

// which 先查 PATH，再查 ~/.convo-dev/tools/<home>/bin。
func which(name, toolHome string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	p := binPath(filepath.Join(toolsRoot(), toolHome, "bin"), name)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p, true
	}
	return "", false
}

// runVersion 跑 "<bin> version" 取一行版本输出（失败返回空）。
func runVersion(path string, args []string) string {
	full := append([]string{path}, args...)
	out, err := exec.Command(full[0], full[1:]...).Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	return line
}

func goTool() tool {
	return tool{
		title: "Go (≥1.22)", binName: "go", verArgs: []string{"version"},
		probe: func() (string, bool) { return which("go", "go") },
		install: func(string) error {
			return fmt.Errorf("Go 需手动安装（convo-dev 本身由 go install 装成，通常已具备）：https://go.dev/dl/")
		},
	}
}

func tinygoTool(ver, urlOverride *string) tool {
	return tool{
		title: "TinyGo", binName: "tinygo", verArgs: []string{"version"},
		probe: func() (string, bool) { return which("tinygo", "tinygo") },
		install: func(mirror string) error {
			v := *ver
			if v == "" {
				v = defaultTinyGoVer
			}
			return installTinyGo(v, resolveURL(urlOverride, tinygoURL(v), mirror))
		},
	}
}

func binaryenTool(ver, urlOverride *string) tool {
	return tool{
		title: "wasm-opt (binaryen)", binName: "wasm-opt", verArgs: []string{"--version"},
		probe: func() (string, bool) { return which("wasm-opt", "binaryen") },
		install: func(mirror string) error {
			v := *ver
			if v == "" {
				v = defaultBinaryenVer
			}
			return installBinaryen(v, resolveURL(urlOverride, binaryenURL(v), mirror))
		},
	}
}

// ---- 下载地址 ----

// tinygoURL：官方 release 资产名形如 tinygo0.41.1.windows-amd64.zip / tinygo0.41.1.linux-amd64.tar.gz。
func tinygoURL(ver string) string {
	osArch := runtime.GOOS + "-" + goarchAlias()
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("https://github.com/tinygo-org/tinygo/releases/download/v%s/tinygo%s.%s.%s", ver, ver, osArch, ext)
}

// binaryenURL：资产名形如 binaryen-version_120-x86_64-windows.tar.gz。
func binaryenURL(ver string) string {
	plat := map[string]string{
		"windows/amd64": "x86_64-windows",
		"linux/amd64":   "x86_64-linux",
		"linux/arm64":   "aarch64-linux",
		"darwin/amd64":  "x86_64-macos",
		"darwin/arm64":  "arm64-macos",
	}[runtime.GOOS+"/"+runtime.GOARCH]
	if plat == "" {
		plat = "x86_64-" + runtime.GOOS // 兜底猜测；404 时用 --binaryen-url 指定
	}
	return fmt.Sprintf("https://github.com/WebAssembly/binaryen/releases/download/version_%s/binaryen-version_%s-%s.tar.gz", ver, ver, plat)
}

func goarchAlias() string {
	if runtime.GOARCH == "386" {
		return "386"
	}
	return runtime.GOARCH
}

// resolveURL：显式覆盖 > 镜像前缀拼接（ghproxy 风形：镜像 + "/" + 原始 URL）> 原始。
func resolveURL(override *string, raw, mirror string) string {
	if override != nil && *override != "" {
		return *override
	}
	if mirror != "" {
		return strings.TrimSuffix(mirror, "/") + "/" + raw
	}
	return raw
}

// ---- doctor / setup / env ----

func runDoctor(args []string) {
	if len(args) > 0 {
		fatalf("doctor 无参数（只做检测，不安装；安装用 convo-dev setup）")
	}
	tgVer, bnVer := "", ""
	tools := []tool{goTool(), tinygoTool(&tgVer, nil), binaryenTool(&bnVer, nil)}
	missing := 0
	fmt.Println("convo 插件开发环境体检")
	for _, t := range tools {
		path, ok := t.probe()
		if !ok {
			fmt.Printf("  ✗ %-18s 未安装   （convo-dev setup 可自动安装）\n", t.title)
			missing++
			continue
		}
		ver := runVersion(path, t.verArgs)
		fmt.Printf("  ✓ %-18s %s\n", t.title, firstToken(ver))
	}
	fmt.Println("\n工具目录（setup 安装位置）：", toolsRoot())
	fmt.Println("shell 集成：eval \"$(convo-dev env)\"")
	if missing > 0 {
		fmt.Printf("\n缺 %d 项：运行 convo-dev setup 自动安装\n", missing)
	}
}

func runSetup(args []string) {
	fs := flag.NewFlagSet("setup", flag.ExitOnError)
	fs.Usage = usage
	tinygoVer := fs.String("tinygo", defaultTinyGoVer, "TinyGo 版本")
	binaryenVer := fs.String("binaryen", defaultBinaryenVer, "binaryen 版本号（如 120）")
	skipTinygo := fs.Bool("skip-tinygo", false, "跳过 TinyGo")
	skipBinaryen := fs.Bool("skip-binaryen", false, "跳过 wasm-opt")
	tinygoURLFlag := fs.String("tinygo-url", "", "TinyGo 下载地址覆盖")
	binaryenURLFlag := fs.String("binaryen-url", "", "binaryen 下载地址覆盖")
	mirror := fs.String("mirror", os.Getenv("CONVO_DEV_MIRROR"), "镜像前缀（ghproxy 风格：前缀+原始URL；也可用 env CONVO_DEV_MIRROR）")
	_ = fs.Parse(args)

	type item struct {
		t    tool
		skip bool
	}
	items := []item{
		{goTool(), false},
		{tinygoTool(tinygoVer, tinygoURLFlag), *skipTinygo},
		{binaryenTool(binaryenVer, binaryenURLFlag), *skipBinaryen},
	}

	failed := 0
	for _, it := range items {
		path, ok := it.t.probe()
		if ok {
			fmt.Printf("✓ %-18s 已安装（%s）\n", it.t.title, firstToken(runVersion(path, it.t.verArgs)))
			continue
		}
		if it.skip {
			fmt.Printf("- %-18s 跳过（--skip）\n", it.t.title)
			continue
		}
		fmt.Printf("==> 安装 %s → %s\n", it.t.title, toolsRoot())
		if err := it.t.install(*mirror); err != nil {
			fmt.Printf("✗ %-18s 安装失败：%v（可用 --tinygo-url/--binaryen-url 指定镜像地址）\n", it.t.title, err)
			failed++
			continue
		}
		if p, ok2 := it.t.probe(); ok2 {
			fmt.Printf("✓ %-18s %s\n", it.t.title, firstToken(runVersion(p, it.t.verArgs)))
		}
	}

	fmt.Printf(`
工具目录：%s
shell 集成（Git Bash / zsh / bash，建议加进 ~/.bashrc 或 ~/.zshrc）：
  eval "$(convo-dev env)"
Windows PowerShell 等效：
  $env:PATH = "$HOME\.convo-dev\tools\tinygo\bin;$HOME\.convo-dev\tools\binaryen\bin;$env:PATH"
（生成的插件项目 build.sh 会自动探测上述目录，不加 PATH 也能构建）
`, toolsRoot())
	if failed > 0 {
		fatalf("%d 项安装失败", failed)
	}
}

// runEnv 输出 shell 集成行。
func runEnv(args []string) {
	if len(args) > 0 {
		fatalf("env 无参数：用法 eval \"$(convo-dev env)\"")
	}
	tinygoHome := filepath.Join(toolsRoot(), "tinygo")
	binaryenBin := filepath.Join(toolsRoot(), "binaryen", "bin")
	// POSIX 形态（Git Bash / Linux / macOS 通用）
	fmt.Printf(`export TINYGOROOT="%s"`+"\n", filepath.ToSlash(tinygoHome))
	fmt.Printf(`export PATH="%s:%s:$PATH"`+"\n", filepath.ToSlash(filepath.Join(tinygoHome, "bin")), filepath.ToSlash(binaryenBin))
	fmt.Printf(`export WASMOPT="%s"`+"\n", filepath.ToSlash(binPath(binaryenBin, "wasm-opt")))
}

// firstToken 取版本串中第一个「像版本号」的词（含数字且有 "."，或纯数字）：
// "tinygo version 0.41.1 windows/amd64 (…)" → "0.41.1"；"go version go1.26.2 …" → "go1.26.2"。
func firstToken(verLine string) string {
	if verLine == "" {
		return "(版本未知)"
	}
	for _, f := range strings.Fields(verLine) {
		if !strings.ContainsAny(f, "0123456789") {
			continue
		}
		if strings.Contains(f, ".") {
			return f
		}
		if isAllDigits(f) {
			return f
		}
	}
	return verLine
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
