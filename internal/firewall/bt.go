package firewall

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BTFirewall 封装宝塔面板防火墙操作
// 宝塔面板使用自己的防火墙管理方式，底层仍是 iptables
// 宝塔的防火墙规则存储在 /www/server/panel/data/ 目录下
// 同时宝塔有自己的 firewalld 或 iptables 管理链
type BTFirewall struct {
	iptablesBin  string
	ip6tablesBin string
	btPanelPath  string
}

const (
	// 宝塔面板默认路径
	btDefaultPath = "/www/server/panel"
	// 宝塔防火墙插件路径
	btFirewallPlugin = "/www/server/panel/plugin/btwaf"
	// 宝塔系统防火墙插件路径
	btSysFirewall = "/www/server/panel/plugin/firewall"
)

// NewBTFirewall 创建宝塔防火墙实例
func NewBTFirewall() (*BTFirewall, error) {
	ipt := findIPTables()
	if ipt == "" {
		return nil, fmt.Errorf("未找到 iptables 命令")
	}
	ip6t := findIP6Tables()
	if ip6t == "" {
		return nil, fmt.Errorf("未找到 ip6tables 命令")
	}

	btPath := btDefaultPath
	if _, err := os.Stat(btPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("未检测到宝塔面板 (路径 %s 不存在)", btPath)
	}

	return &BTFirewall{
		iptablesBin:  ipt,
		ip6tablesBin: ip6t,
		btPanelPath:  btPath,
	}, nil
}

// Name 返回后端名称
func (b *BTFirewall) Name() string {
	return "bt"
}

// CheckAvailable 检查宝塔防火墙是否可用
func (b *BTFirewall) CheckAvailable() error {
	// 检查宝塔面板是否存在
	if _, err := os.Stat(b.btPanelPath); os.IsNotExist(err) {
		return fmt.Errorf("宝塔面板未安装")
	}

	// 检查 iptables 是否可用
	cmd := exec.Command(b.iptablesBin, "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("iptables 不可用: %w", err)
	}

	return nil
}

// ApplyRule 应用规则
// 宝塔环境下，规则插入到 INPUT 链的最前面（在宝塔自身规则之前）
// 同时检查是否有宝塔的 BT-INPUT 链，如果有则也插入
func (b *BTFirewall) ApplyRule(spec RuleSpec) error {
	bin := b.iptablesBin
	if spec.IPv6 {
		bin = b.ip6tablesBin
	}

	comment := fmt.Sprintf("%s%d", CommentPrefix, spec.RuleID)

	protocols := spec.Protocols
	if len(protocols) == 0 {
		if spec.Port != "" {
			protocols = []string{"tcp", "udp"}
		} else {
			protocols = []string{""}
		}
	}

	// 确定要操作的链
	chains := []string{"INPUT"}
	// 检查宝塔是否有自定义链
	if b.chainExists(bin, "BT-INPUT") {
		chains = append(chains, "BT-INPUT")
	}

	for _, chain := range chains {
		for _, proto := range protocols {
			var args []string
			args = append(args, "-I", chain, "1")

			if proto != "" {
				args = append(args, "-p", proto)
			}
			if spec.Port != "" && proto != "" && proto != "icmp" {
				args = append(args, "--dport", normalizePort(spec.Port))
			}

			args = append(args, "-m", "set", "--match-set", spec.SetName, "src")
			args = append(args, "-m", "comment", "--comment", comment)

			if spec.Mode == "white" {
				args = append(args, "-j", "ACCEPT")
			} else {
				args = append(args, "-j", "DROP")
			}

			if err := b.run(bin, args...); err != nil {
				// 自定义链可能不存在，忽略
				if chain != "INPUT" {
					continue
				}
				return err
			}
		}

		// 白名单模式
		if spec.Mode == "white" {
			dropComment := fmt.Sprintf("%s%d_drop", CommentPrefix, spec.RuleID)
			for _, proto := range protocols {
				var args []string
				args = append(args, "-A", chain)
				if proto != "" {
					args = append(args, "-p", proto)
				}
				if spec.Port != "" && proto != "" && proto != "icmp" {
					args = append(args, "--dport", normalizePort(spec.Port))
				}
				args = append(args, "-m", "comment", "--comment", dropComment, "-j", "DROP")
				if err := b.run(bin, args...); err != nil {
					if chain != "INPUT" {
						continue
					}
					return err
				}
			}
		}
	}

	// 将规则持久化到宝塔的配置中（防止宝塔重载时丢失）
	b.persistRule(spec)

	return nil
}

// RemoveRule 移除指定规则
func (b *BTFirewall) RemoveRule(ruleID int) error {
	comment := fmt.Sprintf("%s%d", CommentPrefix, ruleID)
	dropComment := fmt.Sprintf("%s%d_drop", CommentPrefix, ruleID)

	chains := []string{"INPUT"}
	if b.chainExists(b.iptablesBin, "BT-INPUT") {
		chains = append(chains, "BT-INPUT")
	}

	for _, bin := range []string{b.iptablesBin, b.ip6tablesBin} {
		for _, chain := range chains {
			b.removeFromChain(bin, chain, comment)
			b.removeFromChain(bin, chain, dropComment)
		}
	}

	// 清理持久化文件
	b.removePersistRule(ruleID)

	return nil
}

// RemoveAllBAB 移除所有 bab 规则
func (b *BTFirewall) RemoveAllBAB() error {
	chains := []string{"INPUT"}
	if b.chainExists(b.iptablesBin, "BT-INPUT") {
		chains = append(chains, "BT-INPUT")
	}

	for _, bin := range []string{b.iptablesBin, b.ip6tablesBin} {
		for _, chain := range chains {
			b.removeAllFromChain(bin, chain, CommentPrefix)
		}
	}

	// 清理所有持久化文件
	b.removeAllPersistRules()

	return nil
}

// GetRuleCount 获取规则数量
func (b *BTFirewall) GetRuleCount() int {
	count := 0
	chains := []string{"INPUT"}
	if b.chainExists(b.iptablesBin, "BT-INPUT") {
		chains = append(chains, "BT-INPUT")
	}

	for _, bin := range []string{b.iptablesBin, b.ip6tablesBin} {
		for _, chain := range chains {
			cmd := exec.Command(bin, "-L", chain, "-n", "--line-numbers")
			output, err := cmd.Output()
			if err != nil {
				continue
			}
			for _, line := range strings.Split(string(output), "\n") {
				if strings.Contains(line, CommentPrefix) {
					count++
				}
			}
		}
	}
	return count
}

// chainExists 检查链是否存在
func (b *BTFirewall) chainExists(bin, chain string) bool {
	cmd := exec.Command(bin, "-L", chain, "-n")
	return cmd.Run() == nil
}

// removeFromChain 从指定链中移除包含注释的规则
func (b *BTFirewall) removeFromChain(bin, chain, comment string) {
	for {
		lineNum := b.findInChain(bin, chain, comment)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// removeAllFromChain 从指定链中移除所有包含前缀的规则
func (b *BTFirewall) removeAllFromChain(bin, chain, prefix string) {
	for {
		lineNum := b.findInChain(bin, chain, prefix)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// findInChain 在链中查找包含指定字符串的规则行号
func (b *BTFirewall) findInChain(bin, chain, search string) int {
	cmd := exec.Command(bin, "-L", chain, "-n", "--line-numbers")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, search) {
			var num int
			fmt.Sscanf(line, "%d", &num)
			if num > 0 {
				return num
			}
		}
	}
	return 0
}

// persistRule 将规则持久化到宝塔配置目录
// 这样宝塔重载防火墙时可以恢复规则
func (b *BTFirewall) persistRule(spec RuleSpec) {
	persistDir := filepath.Join(b.btPanelPath, "data", "bab_rules")
	os.MkdirAll(persistDir, 0755)

	// 写入规则描述文件
	ruleFile := filepath.Join(persistDir, fmt.Sprintf("rule_%d.conf", spec.RuleID))
	content := fmt.Sprintf("# Block Area Bot Rule %d\n# SetName: %s\n# Mode: %s\n# Port: %s\n# Protocols: %s\n# IPv6: %v\n",
		spec.RuleID, spec.SetName, spec.Mode, spec.Port, strings.Join(spec.Protocols, ","), spec.IPv6)
	os.WriteFile(ruleFile, []byte(content), 0644)
}

// removePersistRule 移除持久化规则文件
func (b *BTFirewall) removePersistRule(ruleID int) {
	persistDir := filepath.Join(b.btPanelPath, "data", "bab_rules")
	ruleFile := filepath.Join(persistDir, fmt.Sprintf("rule_%d.conf", ruleID))
	os.Remove(ruleFile)
}

// removeAllPersistRules 移除所有持久化规则文件
func (b *BTFirewall) removeAllPersistRules() {
	persistDir := filepath.Join(b.btPanelPath, "data", "bab_rules")
	os.RemoveAll(persistDir)
}

// run 执行命令
func (b *BTFirewall) run(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bt/iptables %s 失败: %v, 输出: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// isBTPanel 检测是否安装了宝塔面板
func isBTPanel() bool {
	_, err := os.Stat(btDefaultPath)
	return err == nil
}
