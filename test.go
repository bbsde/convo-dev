// test.go — convo-dev test：静态检查 + 真实网关冒烟（一条命令完成"编译→装→验"闭环）。
//
// 流程：
//   1. 静态检查（无需网关）：plugin.json 校验（name/表名标识符/models）、sync 原语扫描、wasm 存在
//   2. 网关准备：~/.convo-dev/gateway/ 缓存 → 无则按公共 Release latest.json 下载（代理感知）+ sha256 校验
//   3. 冒烟：临时数据目录启动 convo（随机端口）→ 等 health → install-cpk 上传 → echo 实调
//   4. 清理：杀进程、删临时目录（网关缓存保留）
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

const latestJSONURL = "https://github.com/bbsde/convo/releases/latest/download/latest.json"

// latestJSONFallback：仓库 main 分支的 latest.json（CI 每次发布都会提交更新）。
const latestJSONFallback = "https://raw.githubusercontent.com/bbsde/convo/main/latest.json"

var (
 tableNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
 semverRe    = regexp.MustCompile(`\d+\.\d+\.\d+`)
)

type pluginManifest struct {
	Name   string `json:"name"`
	Module string `json:"module"`
	Title  string `json:"title"`
	Tables struct {
		Info     string `json:"info"`
		Accounts string `json:"accounts"`
	} `json:"tables"`
	Models []struct {
		ID string `json:"id"`
	} `json:"models"`
}

func runTest(args []string) {
	projDir, err := os.Getwd()
	if err != nil {
		fatalf("取当前目录失败：%v", err)
	}
	fmt.Printf("==> [1/4] 静态检查（%s）\n", filepath.Base(projDir))
	name := checkStatic(projDir)

	fmt.Println("==> [2/4] 准备 convo 网关")
	gw := ensureGateway()

	fmt.Println("==> [3/4] 启动网关 + 安装插件")
	port, tmpDir, proc := startGateway(gw, name)
	defer func() {
		if proc != nil {
			_ = proc.Process.Kill()
			_, _ = proc.Process.Wait()
		}
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	installCpk(projDir, name, port)

	fmt.Println("==> [4/4] echo 冒烟")
	echoSmoke(name, port)
	fmt.Printf("✅ 全部通过：%s 在真实网关上加载并应答正常\n", name)
}

// ---- 1. 静态检查 ----

func checkStatic(projDir string) string {
	var fails []string
	b, err := os.ReadFile(filepath.Join(projDir, "src", "plugin.json"))
	if err != nil {
		fatalf("读 src/plugin.json 失败：%v（在插件项目根目录跑 convo-dev test）", err)
	}
	var m pluginManifest
	if err := json.Unmarshal(b, &m); err != nil {
		fatalf("src/plugin.json 不是合法 JSON：%v", err)
	}
	if !validName.MatchString(m.Name) {
		fails = append(fails, fmt.Sprintf("name %q 不合法（%s）", m.Name, nameRe))
	}
	for k, tn := range map[string]string{"tables.info": m.Tables.Info, "tables.accounts": m.Tables.Accounts} {
		if tn != "" && !tableNameRe.MatchString(tn) {
			fails = append(fails, fmt.Sprintf("%s = %q 不是合法 SQL 标识符（host 将拒绝加载）", k, tn))
		}
	}
	if len(m.Models) == 0 {
		fails = append(fails, "models 为空（至少声明一个模型）")
	}
	if m.Module != m.Name+".wasm" {
		fails = append(fails, fmt.Sprintf("module = %q，应为 %q", m.Module, m.Name+".wasm"))
	}
	// wasm 存在（build.sh 产物落 src/）
	if _, err := os.Stat(filepath.Join(projDir, "src", m.Name+".wasm")); err != nil {
		fails = append(fails, "src/"+m.Name+".wasm 不存在——先跑 ./build.sh")
	}
	// sync 原语扫描（rules 铁律：TinyGo wasm 内禁用）
	_ = filepath.WalkDir(filepath.Join(projDir, "src"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".go" {
			return err
		}
		fb, rerr := os.ReadFile(p)
		if rerr == nil && bytes.Contains(fb, []byte("sync.")) {
			fails = append(fails, fmt.Sprintf("%s 含 sync 原语（wasm 运行期崩溃，见 docs/rules.md）", filepath.Base(p)))
		}
		return nil
	})
	if len(fails) > 0 {
		for _, f := range fails {
			fmt.Println("  ✗ " + f)
		}
		fatalf("静态检查未通过（%d 项）", len(fails))
	}
	fmt.Printf("  ✓ plugin.json / 表名 / models / wasm / sync 扫描（%s）\n", m.Name)
	return m.Name
}

// ---- 2. 网关缓存/下载 ----

type latestJSON struct {
	Version string `json:"version"`
	Downloads map[string]struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	} `json:"downloads"`
}

func ensureGateway() string {
	// 本地网关优先：CONVO_DEV_GATEWAY=<convo 二进制路径>（开发期用自编网关，
	// 或公共 Release 版本尚无 install-cpk 等新端点时兜底）。
	if p := os.Getenv("CONVO_DEV_GATEWAY"); p != "" {
		if _, err := os.Stat(p); err != nil {
			fatalf("CONVO_DEV_GATEWAY 指向的二进制不存在：%s", p)
		}
		fmt.Printf("  ✓ 本地网关（CONVO_DEV_GATEWAY）：%s\n", p)
		return p
	}
	home, _ := os.UserHomeDir()
	gwDir := filepath.Join(home, ".convo-dev", "gateway")
	_ = os.MkdirAll(gwDir, 0o755)
	key := "linux-x86"
	suffix := ""
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		key, suffix = "windows-x86", ".exe"
	case "linux/amd64":
		key = "linux-x86"
	default:
		fatalf("convo-dev test 暂不支持 %s/%s（网关二进制无对应资产）", runtime.GOOS, runtime.GOARCH)
	}
	// latest.json 优先取 Release 资产（releases/latest/download），404/失败回退 raw main
	// （早期版本未把 latest.json 上传为资产，仓库里的提交版始终可用）。
	client := newHTTPClient(30 * time.Second)
	var body []byte
	for _, u := range []string{latestJSONURL, latestJSONFallback} {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK && len(b) > 0 {
			body = b
			break
		}
	}
	if body == nil {
		fatalf("拉取 convo latest.json 失败（Release 资产与 raw 均不可达）：检查网络/代理")
	}
	var lj latestJSON
	if err := json.Unmarshal(body, &lj); err != nil {
		fatalf("解析 latest.json 失败：%v", err)
	}
	asset, ok := lj.Downloads[key]
	if !ok {
		fatalf("latest.json 无 %s 资产", key)
	}
	bin := filepath.Join(gwDir, "convo-"+lj.Version+"-"+key+suffix)
	if fi, err := os.Stat(bin); err == nil && fi.Size() > 0 {
		fmt.Printf("  ✓ 网关缓存：%s（v%s）\n", bin, lj.Version)
		return bin
	}
	fmt.Printf("  ==> 下载 convo v%s（%s）\n", lj.Version, asset.URL)
	dl, err := newHTTPClient(5 * time.Minute).Get(asset.URL)
	if err != nil {
		fatalf("下载网关失败：%v", err)
	}
	defer dl.Body.Close()
	if dl.StatusCode != http.StatusOK {
		fatalf("下载网关 HTTP %d", dl.StatusCode)
	}
	h := sha256.New()
	f, err := os.Create(bin)
	if err != nil {
		fatalf("写缓存失败：%v", err)
	}
	if _, err := io.Copy(io.MultiWriter(f, h), dl.Body); err != nil {
		f.Close()
		fatalf("下载写入失败：%v", err)
	}
	f.Close()
	if runtime.GOOS != "windows" {
		_ = os.Chmod(bin, 0o755)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != asset.SHA256 {
		_ = os.Remove(bin)
		fatalf("sha256 不符（want %s got %s）——已删缓存，重试", asset.SHA256[:12], got[:12])
	}
	fmt.Printf("  ✓ 下载完成（sha256 校验通过，缓存 %s）\n", filepath.Base(bin))
	return bin
}

// ---- 3/4. 启动、安装、冒烟 ----

func startGateway(bin, pluginName string) (port int, tmpDir string, proc *exec.Cmd) {
	tmpDir, err := os.MkdirTemp("", "convo-dev-test-")
	if err != nil {
		fatalf("建临时目录失败：%v", err)
	}
	_ = os.MkdirAll(filepath.Join(tmpDir, "data"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmpDir, "plugins", "providers"), 0o755)
	port = 20000 + int(time.Now().UnixNano()%20000)
	proc = exec.Command(bin, "-listen", "127.0.0.1:"+strconv.Itoa(port),
		"-log-file", filepath.Join(tmpDir, "convo.log"))
	proc.Env = append(os.Environ(),
		"CONVO_DATA_DIR="+filepath.Join(tmpDir, "data"),
		"CONVO_PLUGINS_DIR="+filepath.Join(tmpDir, "plugins"))
	if err := proc.Start(); err != nil {
		fatalf("启动网关失败：%v", err)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := newHTTPClient(5 * time.Second)
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		if r, err := client.Get(base + "/api/health"); err == nil && r.StatusCode == 200 {
			io.Copy(io.Discard, r.Body)
			r.Body.Close()
			fmt.Printf("  ✓ 网关就绪（%s，临时实例，测完自动清理）\n", base)
			return port, tmpDir, proc
		}
	}
	fatalf("网关 15s 内未就绪——日志 %s", filepath.Join(tmpDir, "convo.log"))
	return
}

// pickCpk 选 dist/ 下语义版本最高的 <name>-<ver>.cpk。
func pickCpk(projDir, name string) string {
	entries, _ := os.ReadDir(filepath.Join(projDir, "dist"))
	var bestVer string
	var bestPath string
	for _, e := range entries {
		m := regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(\d+\.\d+\.\d+)\.cpk$`).FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		if bestVer == "" || semverLess("v"+bestVer, "v"+m[1]) {
			bestVer, bestPath = m[1], filepath.Join(projDir, "dist", e.Name())
		}
	}
	if bestPath == "" {
		fatalf("dist/ 下没有 %s-<ver>.cpk——先跑 ./build.sh", name)
	}
	return bestPath
}

func installCpk(projDir, name string, port int) {
	cpk := pickCpk(projDir, name)
	b, err := os.ReadFile(cpk)
	if err != nil {
		fatalf("读 %s 失败：%v", cpk, err)
	}
	b64 := base64.StdEncoding.EncodeToString(b)
	uid := fmt.Sprintf("cdt%d", time.Now().UnixNano())
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := newHTTPClient(30 * time.Second)
	post := func(obj map[string]any) []byte {
		body, _ := json.Marshal(obj)
		r, err := client.Post(base+"/api/plugins/install-cpk", "application/json", bytes.NewReader(body))
		if err != nil {
			fatalf("上传失败：%v", err)
		}
		defer r.Body.Close()
		rb, _ := io.ReadAll(r.Body)
		if r.StatusCode != 200 {
			fatalf("上传 HTTP %d：%s", r.StatusCode, rb)
		}
		return rb
	}
	const chunk = 512 * 1024
	for off, seq := 0, 0; off < len(b64); off, seq = off+chunk, seq+1 {
		end := off + chunk
		if end > len(b64) {
			end = len(b64)
		}
		post(map[string]any{"upload_id": uid, "seq": seq, "data_b64": b64[off:end]})
	}
	var out struct {
		Name   string `json:"name"`
		Report struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"report"`
	}
	if err := json.Unmarshal(post(map[string]any{"upload_id": uid, "commit": true, "name": filepath.Base(cpk)}), &out); err != nil {
		fatalf("解析安装响应失败：%v", err)
	}
	if out.Report.Status != "loaded" {
		fatalf("插件未加载（status=%s）：%s", out.Report.Status, out.Report.Message)
	}
	fmt.Printf("  ✓ 安装成功：%s（即装即载）\n", filepath.Base(cpk))
}

func echoSmoke(name string, port int) {
	body := []byte(`{"action":"echo","payload":{"convo_dev_smoke":true}}`)
	r, err := newHTTPClient(15 * time.Second).Post(
		fmt.Sprintf("http://127.0.0.1:%d/api/plugins/%s/action", port, name),
		"application/json", bytes.NewReader(body))
	if err != nil {
		fatalf("echo 调用失败：%v", err)
	}
	defer r.Body.Close()
	rb, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || !bytes.Contains(rb, []byte(`"convo_dev_smoke":true`)) {
		fatalf("echo 冒烟失败：HTTP %d %s（期望 200 原样回传）", r.StatusCode, rb)
	}
	fmt.Printf("  ✓ echo 200 原样回传\n")
}
