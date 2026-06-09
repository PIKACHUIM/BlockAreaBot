package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/firewall"
)

// serviceAction 通过 systemctl 执行服务操作
func serviceAction(action string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch action {
	case "start":
		cmd = exec.Command("systemctl", "start", "block-area-bot")
	case "stop":
		cmd = exec.Command("systemctl", "stop", "block-area-bot")
	case "enable":
		cmd = exec.Command("systemctl", "enable", "block-area-bot")
	case "disable":
		cmd = exec.Command("systemctl", "disable", "block-area-bot")
	default:
		return fmt.Errorf("未知操作: %s", action)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 systemctl %s 失败: %v\n%s", action, err, string(output))
	}

	fmt.Printf("操作成功: %s\n", action)
	return nil
}

// showStatus 显示服务状态
func showStatus() error {
	// 服务运行状态
	cmd := exec.Command("systemctl", "is-active", "block-area-bot")
	output, _ := cmd.Output()
	status := "stopped"
	if strings.TrimSpace(string(output)) == "active" {
		status = "running"
	}

	// 开机自启状态
	cmd2 := exec.Command("systemctl", "is-enabled", "block-area-bot")
	output2, _ := cmd2.Output()
	enabled := "disabled"
	if strings.TrimSpace(string(output2)) == "enabled" {
		enabled = "enabled"
	}

	// ipset 和防火墙统计
	ipsetCount := 0
	fwRuleCount := 0
	backendName := "unknown"
	if ipset, err := firewall.NewIPSet(); err == nil {
		ipsetCount = ipset.GetSetCount()
	}

	// 检测防火墙后端
	cfg := config.NewManager()
	if err := cfg.Load(); err == nil {
		c := cfg.GetConfig()
		bt := firewall.BackendType(c.FirewallBackend)
		if bt == "" {
			bt = firewall.DetectBackend()
		}
		if backend, err := firewall.NewBackend(bt); err == nil {
			backendName = backend.Name()
			fwRuleCount = backend.GetRuleCount()
		}
	}

	fmt.Println("Block Area Bot 服务状态")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  运行状态:     %s\n", status)
	fmt.Printf("  开机自启:     %s\n", enabled)
	fmt.Printf("  防火墙后端:   %s\n", backendName)
	fmt.Printf("  ipset 集合:   %d\n", ipsetCount)
	fmt.Printf("  防火墙规则:   %d\n", fwRuleCount)

	return nil
}

// showList 显示总览信息
func showList() error {
	fmt.Println("Block Area Bot 总览")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// 显示服务状态
	_ = showStatus()
	fmt.Println()

	// 显示数据源
	fmt.Println("📦 数据源:")
	cfg := config.NewManager()
	if err := cfg.Load(); err == nil {
		c := cfg.GetConfig()
		if len(c.Repos) == 0 {
			fmt.Println("  (暂无数据源)")
		} else {
			for _, r := range c.Repos {
				fmt.Printf("  [%d] %s  类型: %s  IPv4: %d  IPv6: %d  更新: %s\n",
					r.ID, r.Tag, r.Type, r.IPv4Count, r.IPv6Count,
					r.UpdatedAt.Format("2006-01-02 15:04:05"))
			}
		}
		fmt.Println()

		// 显示规则
		fmt.Println("🛡️  规则:")
		if len(c.Rules) == 0 {
			fmt.Println("  (暂无规则)")
		} else {
			for _, r := range c.Rules {
				proto := "全协议"
				if len(r.Protocols) > 0 {
					proto = strings.Join(r.Protocols, ",")
				}
				port := "全端口"
				if r.Port != "" {
					port = r.Port
				}
				fmt.Printf("  [%d] %s  模式: %s  端口: %s  协议: %s\n",
					r.ID, r.Tag, r.Mode, port, proto)
			}
		}
		fmt.Println()

		// 显示定时任务
		fmt.Println("⏰ 定时任务:")
		if len(c.Crons) == 0 {
			fmt.Println("  (暂无定时任务)")
		} else {
			for _, cr := range c.Crons {
				lastRun := "从未执行"
				if !cr.LastRun.IsZero() {
					lastRun = cr.LastRun.Format("2006-01-02 15:04:05")
				}
				result := cr.LastResult
				if result == "" {
					result = "-"
				}
				fmt.Printf("  %s  间隔: %s  上次: %s  结果: %s\n",
					cr.Tag, cr.Interval, lastRun, result)
			}
		}
	} else {
		fmt.Printf("  加载配置失败: %v\n", err)
	}

	return nil
}