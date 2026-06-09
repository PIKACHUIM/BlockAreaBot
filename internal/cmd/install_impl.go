package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/soulteary/block-area-bot/internal/firewall"
)

const (
	installDir  = "/usr/local/bin"
	binaryName  = "block"
	serviceName = "block-area-bot"
	serviceFile = "/etc/systemd/system/block-area-bot.service"
	configDir   = "/etc/block-area-bot"
	dataDir     = "/var/lib/block-area-bot"
	logDir      = "/var/log/block-area-bot"
)

// systemd 服务文件内容
const serviceContent = `[Unit]
Description=Block Area Bot - 基于地区/IP/ASN的服务器访问屏蔽服务
After=network.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/block daemon
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=block-area-bot

[Install]
WantedBy=multi-user.target
`

// installService 安装系统服务
func installService() error {
	if err := checkRoot(); err != nil {
		return err
	}

	fmt.Println("正在安装 Block Area Bot 系统服务...")
	fmt.Println()

	// 1. 检查二进制文件是否存在
	binaryPath := filepath.Join(installDir, binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// 尝试获取当前可执行文件路径并复制
		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("无法获取当前可执行文件路径: %w", err)
		}
		// 如果当前执行的不是目标路径，复制过去
		if execPath != binaryPath {
			fmt.Printf("  安装二进制文件到 %s ...\n", binaryPath)
			input, err := os.ReadFile(execPath)
			if err != nil {
				return fmt.Errorf("读取可执行文件失败: %w", err)
			}
			if err := os.MkdirAll(installDir, 0755); err != nil {
				return fmt.Errorf("创建目录 %s 失败: %w", installDir, err)
			}
			if err := os.WriteFile(binaryPath, input, 0755); err != nil {
				return fmt.Errorf("写入二进制文件失败: %w", err)
			}
			fmt.Println("  ✓ 二进制文件已安装")
		} else {
			fmt.Println("  ✓ 二进制文件已存在")
		}
	} else {
		fmt.Printf("  ✓ 二进制文件已存在: %s\n", binaryPath)
	}

	// 2. 安装 systemd 服务文件
	fmt.Printf("  安装 systemd 服务文件到 %s ...\n", serviceFile)
	if err := os.WriteFile(serviceFile, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("写入服务文件失败: %w", err)
	}
	fmt.Println("  ✓ 服务文件已安装")

	// 3. 创建配置目录
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	// 如果配置文件不存在，创建默认配置
	configFile := filepath.Join(configDir, "config.json")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		defaultConfig := `{"repos":[],"rules":[],"crons":[],"next_id":1,"next_rule":1}`
		if err := os.WriteFile(configFile, []byte(defaultConfig), 0644); err != nil {
			return fmt.Errorf("创建默认配置文件失败: %w", err)
		}
		fmt.Printf("  ✓ 配置文件已创建: %s\n", configFile)
	} else {
		fmt.Printf("  ✓ 配置文件已存在: %s\n", configFile)
	}

	// 4. 创建数据目录和日志目录
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	fmt.Printf("  ✓ 数据目录: %s\n", dataDir)

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	fmt.Printf("  ✓ 日志目录: %s\n", logDir)

	// 5. 重新加载 systemd
	cmd := exec.Command("systemctl", "daemon-reload")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败: %v\n%s", err, string(output))
	}
	fmt.Println("  ✓ systemd 已重新加载")

	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println(" 安装完成!")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println(" 启动服务:  block start")
	fmt.Println(" 开机自启:  block enable")
	fmt.Println(" 查看状态:  block status")
	fmt.Println()

	return nil
}

// uninstallService 卸载服务
func uninstallService(all bool) error {
	if err := checkRoot(); err != nil {
		return err
	}

	fmt.Println("正在卸载 Block Area Bot...")
	fmt.Println()

	// 1. 停止并禁用服务
	fmt.Println("  停止服务...")
	exec.Command("systemctl", "stop", "block-area-bot").Run()
	exec.Command("systemctl", "disable", "block-area-bot").Run()
	fmt.Println("  ✓ 服务已停止并禁用")

	// 2. 清除防火墙规则
	fmt.Println("  清除防火墙规则...")
	if ipset, err := firewall.NewIPSet(); err == nil {
		ipset.DestroyAllBAB()
	}
	if backend, err := firewall.NewBackend(firewall.DetectBackend()); err == nil {
		backend.RemoveAllBAB()
	}
	fmt.Println("  ✓ 防火墙规则已清除")

	// 3. 移除 systemd 服务文件
	if _, err := os.Stat(serviceFile); err == nil {
		os.Remove(serviceFile)
		exec.Command("systemctl", "daemon-reload").Run()
		fmt.Printf("  ✓ 服务文件已移除: %s\n", serviceFile)
	}

	// 4. 判断是否卸载 CLI 和数据
	removeCLI := all
	if !all {
		// 交互式询问用户
		fmt.Println()
		fmt.Print("  是否同时卸载 CLI 工具和所有数据? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "y" || answer == "yes" {
			removeCLI = true
		}
	}

	if removeCLI {
		// 移除二进制文件
		binaryPath := filepath.Join(installDir, binaryName)
		if _, err := os.Stat(binaryPath); err == nil {
			os.Remove(binaryPath)
			fmt.Printf("  ✓ 二进制文件已移除: %s\n", binaryPath)
		}

		// 移除配置目录
		if _, err := os.Stat(configDir); err == nil {
			os.RemoveAll(configDir)
			fmt.Printf("  ✓ 配置目录已移除: %s\n", configDir)
		}

		// 移除数据目录
		if _, err := os.Stat(dataDir); err == nil {
			os.RemoveAll(dataDir)
			fmt.Printf("  ✓ 数据目录已移除: %s\n", dataDir)
		}

		// 移除日志目录
		if _, err := os.Stat(logDir); err == nil {
			os.RemoveAll(logDir)
			fmt.Printf("  ✓ 日志目录已移除: %s\n", logDir)
		}

		fmt.Println()
		fmt.Println("  Block Area Bot 已完全卸载。")
	} else {
		fmt.Println()
		fmt.Println("  服务已卸载，CLI 工具和数据已保留。")
		fmt.Printf("  二进制: %s\n", filepath.Join(installDir, binaryName))
		fmt.Printf("  配置:   %s\n", configDir)
		fmt.Printf("  数据:   %s\n", dataDir)
	}

	return nil
}
