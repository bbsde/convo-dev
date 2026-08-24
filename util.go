// util.go — 共享工具：文件树复制、下载、解压、镜像与杂项。
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func regexpMust(pat string) *regexp.Regexp { return regexp.MustCompile(pat) }

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}

// copyTree 递归复制目录（或单文件）。
func copyTree(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst, st.Mode())
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, _ := d.Info()
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// downloadTo 下载到目标文件（打印大小），返回错误。
func downloadTo(url, dst string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d（%s）", resp.StatusCode, url)
	}
	if resp.ContentLength > 0 {
		fmt.Printf("    下载中：%s（%.1f MB）\n", url, float64(resp.ContentLength)/1024/1024)
	} else {
		fmt.Printf("    下载中：%s\n", url)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// extractArchive 解压 .zip / .tar.gz 到 dst 目录，返回解压出的顶层目录名（如 "tinygo"、"binaryen-version_120"）。
func extractArchive(archive, dst string) (string, error) {
	switch {
	case strings.HasSuffix(archive, ".zip"):
		return extractZip(archive, dst)
	default:
		return extractTarGz(archive, dst)
	}
}

func extractZip(archive, dst string) (string, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	top := ""
	for _, f := range zr.File {
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), "./")
		if top == "" && strings.Contains(name, "/") {
			top = name[:strings.Index(name, "/")]
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return "", err
		}
		out.Close()
		rc.Close()
	}
	if top == "" {
		return "", fmt.Errorf("zip 内容无顶层目录")
	}
	return top, nil
}

func extractTarGz(archive, dst string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	top := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if top == "" && strings.Contains(name, "/") {
			top = name[:strings.Index(name, "/")]
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
		}
	}
	if top == "" {
		return "", fmt.Errorf("tar.gz 内容无顶层目录")
	}
	return top, nil
}

// installTool 通用安装：下载 → 解压 → 把顶层目录就位为 ~/.convo-dev/tools/<slot> → 校验可执行。
// slotName 传 "tinygo" / "binaryen"；binCheck 传要校验存在的 bin 下文件名。
func installTool(slotName, url, binCheck string) error {
	tmp, err := os.MkdirTemp("", "convo-dev-setup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// 保留 URL 后缀（.zip/.gz）供解压器分发；截掉 query 串
	pure := url
	if i := strings.IndexByte(pure, '?'); i >= 0 {
		pure = pure[:i]
	}
	archive := filepath.Join(tmp, "dl"+filepath.Ext(pure))
	if err := downloadTo(url, archive); err != nil {
		return err
	}
	fmt.Println("    解压…")
	top, err := extractArchive(archive, tmp)
	if err != nil {
		return err
	}

	slot := filepath.Join(toolsRoot(), slotName)
	if _, err := os.Stat(slot); err == nil {
		_ = os.RemoveAll(slot) // 重装替换旧版本
	}
	if err := os.MkdirAll(filepath.Dir(slot), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(tmp, top), slot); err != nil {
		return err
	}
	bin := binPath(filepath.Join(slot, "bin"), binCheck)
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("安装后未找到 %s", bin)
	}
	return nil
}

func installTinyGo(ver, url string) error {
	if err := installTool("tinygo", url, "tinygo"); err != nil {
		return err
	}
	fmt.Printf("    TinyGo v%s 安装完成\n", ver)
	return nil
}

func installBinaryen(ver, url string) error {
	if err := installTool("binaryen", url, "wasm-opt"); err != nil {
		return err
	}
	fmt.Printf("    binaryen version_%s 安装完成\n", ver)
	return nil
}
