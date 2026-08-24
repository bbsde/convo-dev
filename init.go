// init.go — convo-dev init：生成插件项目（远程拉取本仓 template/ 或本地模板）。
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runInit 生成插件项目。
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	fs.Usage = usage
	dir := fs.String("d", "", "目标目录（默认 ./<name>）")
	ref := fs.String("ref", "main", "模板分支/tag")
	url := fs.String("url", "", "完整 tarball 地址（覆盖默认）")
	tplDir := fs.String("template", "", "本地模板目录（跳过远程拉取）")
	_ = fs.Parse(args)

	name := fs.Arg(0)
	if !validName.MatchString(name) {
		fatalf("用法：convo-dev init <name>；name 须匹配 %s", nameRe)
	}
	dest := *dir
	if dest == "" {
		dest = name
	}
	if st, err := os.Stat(dest); err == nil {
		if !st.IsDir() {
			fatalf("目标已存在且不是目录：%s", dest)
		}
		ents, _ := os.ReadDir(dest)
		if len(ents) > 0 {
			fatalf("目标目录非空：%s", dest)
		}
	}

	// 1. 取模板（本地 or 远程 tarball）
	tmp, err := os.MkdirTemp("", "convo-dev-tpl-")
	if err != nil {
		fatalf("创建临时目录失败：%v", err)
	}
	defer os.RemoveAll(tmp)

	if *tplDir != "" {
		if err := copyTree(*tplDir, tmp); err != nil {
			fatalf("复制本地模板失败：%v", err)
		}
		fmt.Printf("==> 模板（本地）：%s\n", *tplDir)
	} else {
		dlURL := *url
		if dlURL == "" {
			kind := "heads"
			if regexpMust(`^v\d`).MatchString(*ref) {
				kind = "tags"
			}
			dlURL = fmt.Sprintf(downloadFmt, repoName, kind, *ref)
		}
		fmt.Printf("==> 拉取模板：%s\n", dlURL)
		if err := fetchTemplateTree(dlURL, tmp); err != nil {
			fatalf("拉取模板失败：%v\n  （网络不通？可用 --template <本地目录> 或 --url <镜像地址>）", err)
		}
	}
	requireTemplate(tmp)

	// 2. 落位：src/ docs/ build.sh → 目标项目；dist/ 空目录
	fmt.Printf("==> 生成插件 %s → %s\n", name, dest)
	for _, part := range []string{"src", "docs", "build.sh"} {
		if err := copyTree(filepath.Join(tmp, part), filepath.Join(dest, part)); err != nil {
			fatalf("写入 %s 失败：%v", part, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dest, "dist"), 0o755); err != nil {
		fatalf("创建 dist/ 失败：%v", err)
	}
	_ = os.WriteFile(filepath.Join(dest, "dist", ".gitkeep"), nil, 0o644)

	// 3. 占位符替换
	exts := map[string]bool{
		".go": true, ".json": true, ".sql": true, ".html": true,
		".md": true, ".sh": true, ".mod": true,
	}
	n := 0
	_ = filepath.WalkDir(dest, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !exts[strings.ToLower(filepath.Ext(path))] {
			return err
		}
		if strings.HasSuffix(path, ".sh") {
			_ = os.Chmod(path, 0o755)
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil || !strings.Contains(string(b), placeholder) {
			return rerr
		}
		werr := os.WriteFile(path, []byte(strings.ReplaceAll(string(b), placeholder, name)), 0o644)
		if werr != nil {
			return werr
		}
		n++
		return nil
	})
	fmt.Printf("==> 占位符替换：%d 个文件\n", n)

	fmt.Printf(`
✅ 插件已生成：%s

    %s/
    ├── build.sh     构建脚本（产物 → dist/）
    ├── dist/        构建产物（wasm / cpk）
    ├── docs/        项目文档（README + NOTES 逆向笔记）
    └── src/         源码（main.go / plugin.json / migrations / ui）

下一步：
  1. 编辑 src/plugin.json：真实模型清单（models[].id）、标题、描述
  2. 按目标平台协议改写 src/main.go 的 verifyKey / handleChat
  3. 环境（首次）：convo-dev setup     # 缺 TinyGo/wasm-opt 时自动安装
  4. 构建：cd %s && ./build.sh
  5. 安装到 convo 网关调试 / 打包分发：https://github.com/bbsde/convo-dev → docs/testing.md

硬性约定速查（务必遵守）：docs/rules.md —— echo action 必须保留 / wasm 内禁 sync
原语 / 不整份 JSON 树化 / 插件 UI 一律绝对 URL（window.__CONVO_ABS__）
`, dest, dest, dest)
}

// requireTemplate 校验模板目录齐备。
func requireTemplate(dir string) {
	for _, p := range []string{filepath.Join(dir, "src", "main.go"), filepath.Join(dir, "src", "plugin.json")} {
		if _, err := os.Stat(p); err != nil {
			fatalf("模板不完整（缺 %s）——检查 --template/--ref 指向", p)
		}
	}
}

// fetchTemplateTree 下载仓库 tarball，抽出 template/ 子树写入 dst。
func fetchTemplateTree(url, dst string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	prefix := "" // "convo-dev-<ref>/template/"
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if prefix == "" {
			i := strings.Index(hdr.Name, "/template/")
			if i < 0 {
				continue
			}
			prefix = hdr.Name[:i+len("/template/")]
		}
		if !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		rel := strings.TrimPrefix(hdr.Name, prefix)
		if rel == "" || strings.Contains(rel, "..") {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	if prefix == "" {
		return fmt.Errorf("tarball 中未找到 template/ 子树")
	}
	return nil
}
