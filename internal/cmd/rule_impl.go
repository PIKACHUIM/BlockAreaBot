package cmd

import (
	"fmt"
	"strings"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/firewall"
	"github.com/soulteary/block-area-bot/internal/rule"
)

// initBackend 初始化防火墙后端（CLI 命令共用）
func initBackend(cfg *config.Manager) firewall.Backend {
	c := cfg.GetConfig()
	backendType := firewall.BackendType(c.FirewallBackend)
	if backendType == "" {
		backendType = firewall.DetectBackend()
	}
	backend, _ := firewall.NewBackend(backendType)
	return backend
}

// ruleBan 添加屏蔽规则
func ruleBan(tag, mode, port string, tcp, udp, icmp bool) error {
	if err := checkRoot(); err != nil {
		return err
	}

	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 构建协议列表
	var protocols []string
	if tcp {
		protocols = append(protocols, "tcp")
	}
	if udp {
		protocols = append(protocols, "udp")
	}
	if icmp {
		protocols = append(protocols, "icmp")
	}

	// 初始化 firewall
	ipset, _ := firewall.NewIPSet()
	backend := initBackend(cfg)

	ruleMgr := rule.NewManager(cfg, ipset, backend)
	if err := ruleMgr.Ban(tag, mode, port, protocols); err != nil {
		return err
	}

	// 显示结果
	if mode == "" {
		mode = "black"
	}
	proto := "全协议"
	if len(protocols) > 0 {
		proto = strings.Join(protocols, ",")
	}
	portStr := "全端口"
	if port != "" {
		portStr = port
	}

	fmt.Printf("✓ 屏蔽规则添加成功\n")
	fmt.Printf("  数据源: %s\n", tag)
	fmt.Printf("  模式:   %s\n", mode)
	fmt.Printf("  端口:   %s\n", portStr)
	fmt.Printf("  协议:   %s\n", proto)

	return nil
}

// ruleDel 删除屏蔽规则
func ruleDel(tagOrID, port string, tcp, udp, icmp bool) error {
	if err := checkRoot(); err != nil {
		return err
	}

	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 构建协议列表
	var protocols []string
	if tcp {
		protocols = append(protocols, "tcp")
	}
	if udp {
		protocols = append(protocols, "udp")
	}
	if icmp {
		protocols = append(protocols, "icmp")
	}

	ipset, _ := firewall.NewIPSet()
	backend := initBackend(cfg)

	ruleMgr := rule.NewManager(cfg, ipset, backend)
	deleted, err := ruleMgr.Del(tagOrID, port, protocols)
	if err != nil {
		return err
	}

	fmt.Printf("✓ 已删除 %d 条规则\n", len(deleted))
	for _, r := range deleted {
		fmt.Printf("  - 规则 %d: %s (模式: %s)\n", r.ID, r.Tag, r.Mode)
	}

	return nil
}