package firewall

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// UFW 封装 ufw 防火墙操作
// ufw 底层仍然使用 iptables，但通过 ufw 命令管理规则
// 本实现使用 ufw route/insert 命令配合 ipset 实现屏蔽
type UFW struct {
	ufwBin       string
	iptablesBin  string
	ip6tablesBin string
}

// NewUFW 创建 UFW 实例
func NewUFW() (*UFW, error) {
	ufwBin, err := exec.LookPath("ufw")
	if err != nil {
		return nil, fmt.Errorf("未找到 ufw 命令: %w", err)
	}

	// ufw 底层仍需要 iptables 来配合 ipset
	ipt := findIPTables()
	ip6t := findIP6Tables()

	return &UFW{
		ufwBin:       ufwBin,
		iptablesBin:  ipt,
		ip6tablesBin: ip6t,
	}, nil
}

// Name 返回后端名称
func (u *UFW) Name() string {
	return "ufw"
}

// CheckAvailable 检查 ufw 是否可用
func (u *UFW) CheckAvailable() error {
	cmd := exec.Command(u.ufwBin, "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw 不可用: %v, 输出: %s", err, string(output))
	}
	if strings.Contains(string(output), "inactive") {
		return fmt.Errorf("ufw 未启用，请先运行 'ufw enable'")
	}
	return nil
}

// ApplyRule 通过 ufw before.rules 配合 iptables 应用规则
// ufw 不直接支持 ipset，所以我们通过 iptables 直接插入规则到 ufw 管理的链中
func (u *UFW) ApplyRule(spec RuleSpec) error {
	bin := u.iptablesBin
	if spec.IPv6 {
		bin = u.ip6tablesBin
	}
	if bin == "" {
		return fmt.Errorf("未找到 iptables 命令")
	}

	comment := fmt.Sprintf("%s%d", CommentPrefix, spec.RuleID)

	// ufw 使用 ufw-before-input 链，我们插入到该链
	chain := "ufw-before-input"

	protocols := spec.Protocols
	if len(protocols) == 0 {
		if spec.Port != "" {
			protocols = []string{"tcp", "udp"}
		} else {
			protocols = []string{""}
		}
	}

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

		if err := u.runIPT(bin, args...); err != nil {
			return err
		}
	}

	// 白名单模式：添加 DROP ALL 规则
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
			if err := u.runIPT(bin, args...); err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveRule 移除指定规则
func (u *UFW) RemoveRule(ruleID int) error {
	comment := fmt.Sprintf("%s%d", CommentPrefix, ruleID)
	dropComment := fmt.Sprintf("%s%d_drop", CommentPrefix, ruleID)
	chain := "ufw-before-input"

	for _, bin := range []string{u.iptablesBin, u.ip6tablesBin} {
		if bin == "" {
			continue
		}
		u.removeFromChain(bin, chain, comment)
		u.removeFromChain(bin, chain, dropComment)
	}
	return nil
}

// RemoveAllBAB 移除所有 bab 规则
func (u *UFW) RemoveAllBAB() error {
	chain := "ufw-before-input"
	for _, bin := range []string{u.iptablesBin, u.ip6tablesBin} {
		if bin == "" {
			continue
		}
		u.removeAllFromChain(bin, chain, CommentPrefix)
	}
	return nil
}

// GetRuleCount 获取规则数量
func (u *UFW) GetRuleCount() int {
	count := 0
	chain := "ufw-before-input"
	for _, bin := range []string{u.iptablesBin, u.ip6tablesBin} {
		if bin == "" {
			continue
		}
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
	return count
}

// removeFromChain 从指定链中移除包含注释的规则
func (u *UFW) removeFromChain(bin, chain, comment string) {
	for {
		lineNum := u.findInChain(bin, chain, comment)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// removeAllFromChain 从指定链中移除所有包含前缀的规则
func (u *UFW) removeAllFromChain(bin, chain, prefix string) {
	for {
		lineNum := u.findInChain(bin, chain, prefix)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// findInChain 在链中查找包含指定字符串的规则行号
func (u *UFW) findInChain(bin, chain, search string) int {
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

// runIPT 执行 iptables 命令
func (u *UFW) runIPT(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ufw/iptables %s 失败: %v, 输出: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// isUFWAvailable 检测 ufw 是否可用
func isUFWAvailable() bool {
	if _, err := exec.LookPath("ufw"); err != nil {
		return false
	}
	// 检查 ufw 是否处于活动状态
	cmd := exec.Command("ufw", "status")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "active") && !strings.Contains(string(output), "inactive")
}

// PersistUFWRules 将规则持久化到 ufw before.rules 文件
// 这确保 ufw reload 后规则仍然存在
func (u *UFW) PersistUFWRules(ipv4Rules, ipv6Rules []string) error {
	// 写入 /etc/ufw/before.rules 的自定义部分
	babMarker := "# BAB-RULES-START"
	babEnd := "# BAB-RULES-END"

	for _, filePath := range []string{"/etc/ufw/before.rules", "/etc/ufw/before6.rules"} {
		var rules []string
		if filePath == "/etc/ufw/before.rules" {
			rules = ipv4Rules
		} else {
			rules = ipv6Rules
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		fileContent := string(content)

		// 移除旧的 BAB 规则块
		if idx := strings.Index(fileContent, babMarker); idx >= 0 {
			endIdx := strings.Index(fileContent, babEnd)
			if endIdx >= 0 {
				fileContent = fileContent[:idx] + fileContent[endIdx+len(babEnd)+1:]
			}
		}

		// 如果有新规则，插入到 COMMIT 之前
		if len(rules) > 0 {
			babBlock := babMarker + "\n"
			for _, r := range rules {
				babBlock += r + "\n"
			}
			babBlock += babEnd + "\n"

			commitIdx := strings.LastIndex(fileContent, "COMMIT")
			if commitIdx >= 0 {
				fileContent = fileContent[:commitIdx] + babBlock + fileContent[commitIdx:]
			}
		}

		if err := os.WriteFile(filePath, []byte(fileContent), 0640); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", filePath, err)
		}
	}

	return nil
}
