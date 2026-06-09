package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// GitHub 仓库信息
	repoOwner = "PIKACHUIM"
	repoName  = "BlockAreaBot"
	// 二进制文件安装路径
	binaryInstallPath = "/usr/local/bin/block"
)

// githubRelease GitHub Release API 响应结构
type githubRelease struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Assets  []githubAsset  `json:"assets"`
	Body    string         `json:"body"`
}

// githubAsset Release 资源文件
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// runUpdate 执行更新流程
func runUpdate(version string, force bool) error {
	if err := checkRoot(); err != nil {
		return err
	}

	fmt.Println("Block Area Bot 自动更新")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. 获取目标版本信息
	release, err := fetchRelease(version)
	if err != nil {
		return fmt.Errorf("获取版本信息失败: %w", err)
	}
	fmt.Printf("  目标版本: %s\n", release.TagName)

	// 2. 查找匹配当前架构的资源文件
	arch := runtime.GOARCH
	assetName := fmt.Sprintf("block-linux-%s", arch)
	var targetAsset *githubAsset
	for i, asset := range release.Assets {
		if strings.Contains(asset.Name, assetName) && strings.HasSuffix(asset.Name, ".tar.gz") {
			targetAsset = &release.Assets[i]
			break
		}
	}
	// 也尝试匹配不带版本号的格式
	if targetAsset == nil {
		for i, asset := range release.Assets {
			if strings.Contains(asset.Name, arch) && strings.HasSuffix(asset.Name, ".tar.gz") {
				targetAsset = &release.Assets[i]
				break
			}
		}
	}
	if targetAsset == nil {
		return fmt.Errorf("未找到适用于当前架构 (%s) 的安装包", arch)
	}
	fmt.Printf("  安装包:   %s (%.2f MB)\n", targetAsset.Name, float64(targetAsset.Size)/1024/1024)

	// 3. 检查是否需要更新
	if !force {
		currentVersion := getCurrentVersion()
		if currentVersion != "" && currentVersion == release.TagName {
			fmt.Printf("\n  当前已是最新版本 (%s)，无需更新\n", currentVersion)
			fmt.Println("  使用 --force 强制更新")
			return nil
		}
		if currentVersion != "" {
			fmt.Printf("  当前版本: %s\n", currentVersion)
		}
	}

	fmt.Println()

	// 4. 停止服务并禁用规则
	serviceWasRunning := isServiceRunning()
	if serviceWasRunning {
		fmt.Println("  [1/5] 停止服务...")
		if err := exec.Command("systemctl", "stop", "block-area-bot").Run(); err != nil {
			fmt.Printf("  警告: 停止服务失败: %v\n", err)
		} else {
			fmt.Println("        ✓ 服务已停止，防火墙规则已清除")
		}
	} else {
		fmt.Println("  [1/5] 服务未运行，跳过停止步骤")
	}

	// 5. 下载新版本
	fmt.Printf("  [2/5] 下载新版本...\n")
	tmpDir, err := os.MkdirTemp("", "block-area-bot-update-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, targetAsset.Name)
	if err := downloadFile(targetAsset.BrowserDownloadURL, tarPath); err != nil {
		return fmt.Errorf("下载失败: %w", err)
	}
	fmt.Println("        ✓ 下载完成")

	// 6. 解压并替换二进制文件
	fmt.Println("  [3/5] 解压并安装...")
	extractDir := filepath.Join(tmpDir, "extract")
	os.MkdirAll(extractDir, 0755)
	if err := extractTarGz(tarPath, extractDir); err != nil {
		return fmt.Errorf("解压失败: %w", err)
	}

	// 查找解压后的二进制文件
	newBinary := findBinary(extractDir)
	if newBinary == "" {
		return fmt.Errorf("解压后未找到可执行文件")
	}

	// 备份旧文件
	backupPath := binaryInstallPath + ".bak"
	if _, err := os.Stat(binaryInstallPath); err == nil {
		os.Rename(binaryInstallPath, backupPath)
	}

	// 复制新文件
	if err := copyFile(newBinary, binaryInstallPath); err != nil {
		// 恢复备份
		if _, err2 := os.Stat(backupPath); err2 == nil {
			os.Rename(backupPath, binaryInstallPath)
		}
		return fmt.Errorf("安装新版本失败: %w", err)
	}
	os.Chmod(binaryInstallPath, 0755)
	os.Remove(backupPath)
	fmt.Println("        ✓ 二进制文件已更新")

	// 7. 更新 systemd service 文件（如果包中有）
	fmt.Println("  [4/5] 更新服务配置...")
	serviceFile := filepath.Join(extractDir, "dist", "block-area-bot.service")
	if _, err := os.Stat(serviceFile); err == nil {
		if err := copyFile(serviceFile, "/etc/systemd/system/block-area-bot.service"); err == nil {
			exec.Command("systemctl", "daemon-reload").Run()
			fmt.Println("        ✓ 服务配置已更新")
		}
	} else {
		fmt.Println("        - 无需更新服务配置")
	}

	// 8. 重新启动服务
	fmt.Println("  [5/5] 恢复服务...")
	if serviceWasRunning {
		if err := exec.Command("systemctl", "start", "block-area-bot").Run(); err != nil {
			fmt.Printf("        ⚠ 启动服务失败: %v\n", err)
			fmt.Println("        请手动运行: block start")
		} else {
			fmt.Println("        ✓ 服务已启动，防火墙规则已恢复")
		}
	} else {
		fmt.Println("        - 服务之前未运行，跳过启动")
	}

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  ✓ 更新完成！当前版本: %s\n", release.TagName)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// fetchRelease 获取 GitHub Release 信息
func fetchRelease(version string) (*githubRelease, error) {
	var url string
	if version == "" {
		// 默认获取 beta release
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/beta", repoOwner, repoName)
	} else {
		// 指定版本
		version = strings.TrimPrefix(version, "v")
		// 先尝试 tag 为 v+version
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/v%s", repoOwner, repoName, version)
	}

	release, err := fetchReleaseFromURL(url)
	if err != nil && version != "" {
		// 尝试不带 v 前缀
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", repoOwner, repoName, version)
		release, err = fetchReleaseFromURL(url)
	}
	if err != nil && version == "" {
		// beta 不存在，尝试 latest
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)
		release, err = fetchReleaseFromURL(url)
	}

	return release, err
}

// fetchReleaseFromURL 从指定 URL 获取 Release 信息
func fetchReleaseFromURL(url string) (*githubRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: 版本不存在或网络错误", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &release, nil
}

// downloadFile 下载文件到指定路径
func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，HTTP 状态码: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// extractTarGz 解压 tar.gz 文件
func extractTarGz(tarPath, destDir string) error {
	cmd := exec.Command("tar", "-xzf", tarPath, "-C", destDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar 解压失败: %v, 输出: %s", err, string(output))
	}
	return nil
}

// findBinary 在目录中查找可执行的 block 二进制文件
func findBinary(dir string) string {
	// 优先查找名为 "block" 的文件
	candidates := []string{
		filepath.Join(dir, "block"),
		filepath.Join(dir, "block-area-bot"),
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}

	// 递归查找
	var found string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "block" || name == "block-area-bot" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// 确保目标目录存在
	os.MkdirAll(filepath.Dir(dst), 0755)

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// getCurrentVersion 获取当前安装的版本号
func getCurrentVersion() string {
	cmd := exec.Command(binaryInstallPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// 解析版本输出
	ver := strings.TrimSpace(string(output))
	// 可能输出格式为 "block-area-bot version v1.0.0" 或 "v1.0.0"
	parts := strings.Fields(ver)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ver
}

// isServiceRunning 检查服务是否正在运行
func isServiceRunning() bool {
	cmd := exec.Command("systemctl", "is-active", "block-area-bot")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "active"
}
