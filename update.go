// update.go — convo-dev update：检查 GitHub 最新 tag 并自更新。
package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title string `xml:"title"`
}

// fetchLatestTag 读仓库 tags.atom（无需认证、无 API 限流），返回最高的纯 vX.Y.Z
// （排除 pdk/ 子模块 tag）。feed 含最近发布的 tag，取语义最高即可。
func fetchLatestTag() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get("https://github.com/bbsde/" + repoName + "/tags.atom")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	var feed atomFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		return "", err
	}
	best := ""
	for _, e := range feed.Entries {
		if !semverTagRe.MatchString(e.Title) {
			continue
		}
		if best == "" || semverLess(best, e.Title) {
			best = e.Title
		}
	}
	if best == "" {
		return "", fmt.Errorf("feed 中未找到 CLI 版本（vX.Y.Z）")
	}
	return best, nil
}

// semverLess 比较 vA.B.C < vD.E.F。
func semverLess(a, b string) bool {
	var pa, pb [3]int
	fmt.Sscanf(a, "v%d.%d.%d", &pa[0], &pa[1], &pa[2])
	fmt.Sscanf(b, "v%d.%d.%d", &pb[0], &pb[1], &pb[2])
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

// runUpdate 检查最新版并 go install 自更新。
func runUpdate(args []string) {
	cur := selfVersion()
	if cur == "" {
		fmt.Println("当前 (devel)——本地构建，无法比对，直接装最新")
	} else {
		fmt.Printf("当前 %s\n", cur)
	}
	latest, err := fetchLatestTag()
	if err != nil {
		fatalf("查询最新版本失败：%v\n  网络不通可稍后重试，或手动：go install github.com/bbsde/%s@latest", err, repoName)
	}
	fmt.Printf("最新 %s\n", latest)
	if cur == latest {
		fmt.Println("✅ 已是最新")
		return
	}
	fmt.Println("==> go install（GOPROXY=direct 直连 GitHub，绕开 sumdb 对新 tag 的索引延迟）")
	cmd := exec.Command("go", "install", "github.com/bbsde/"+repoName+"@"+latest)
	cmd.Env = append(os.Environ(), "GOPROXY=direct", "GOSUMDB=off")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("go install 失败：%v", err)
	}
	fmt.Printf("✅ 已更新到 %s（convo-dev version 验证）\n", latest)
}
