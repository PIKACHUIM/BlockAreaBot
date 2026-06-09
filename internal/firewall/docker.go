package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

// DockerFirewall 封装 Docker 环境下的防火墙操作
// Docker 会创建自己的 iptables 链（DOCKER、DOCKER-USER 等）
// 在 Docker 环境中，应将规则插入到 DOCKER-USER 链，而非 INPUT 链
// 因为 Docker 的 FORWARD 链会绕过 INPUT 链
type DockerFirewall struct {
	iptablesBin  string
	ip6tablesBin string
}

// NewDockerFirewall 创建 Docker 防火墙实例
func NewDockerFirewall() (*DockerFirewall, error) {
	ipt := findIPTables()
	if ipt == "" {
		return nil, fmt.Errorf("未找到 iptables 命令")
	}
	ip6t := findIP6Tables()
	if ip6t == "" {
		return nil, fmt.Errorf("未找到 ip6tables 命令")
	}

	return &DockerFirewall{
		iptablesBin:  ipt,
		ip6tablesBin: ip6t,
	}, nil
}

// Name 返回后端名称
func (d *DockerFirewall) Name() string {
	return "docker"
}

// CheckAvailable 检查 Docker 防火墙环境是否可用
func (d *DockerFirewall) CheckAvailable() error {
	// 检查 DOCKER-USER 链是否存在
	cmd := exec.Command(d.iptablesBin, "-L", "DOCKER-USER", "-n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("DOCKER-USER 链不存在，Docker 环境可能未正确配置: %v, 输出: %s", err, string(output))
	}
	return nil
}

// ApplyRule 应用规则到 Docker 环境
// 同时在 INPUT 链和 DOCKER-USER 链中添加规则
func (d *DockerFirewall) ApplyRule(spec RuleSpec) error {
	bin := d.iptablesBin
	if spec.IPv6 {
		bin = d.ip6tablesBin
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

	// 需要在两个链中添加规则：INPUT（主机访问）和 DOCKER-USER（容器访问）
	chains := []string{"INPUT", "DOCKER-USER"}

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

			if err := d.run(bin, args...); err != nil {
				// DOCKER-USER 链可能不支持 IPv6，忽略错误
				if chain == "DOCKER-USER" && spec.IPv6 {
					continue
				}
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
				if err := d.run(bin, args...); err != nil {
					if chain == "DOCKER-USER" && spec.IPv6 {
						continue
					}
					return err
				}
			}
		}
	}

	return nil
}

// RemoveRule 移除指定规则
func (d *DockerFirewall) RemoveRule(ruleID int) error {
	comment := fmt.Sprintf("%s%d", CommentPrefix, ruleID)
	dropComment := fmt.Sprintf("%s%d_drop", CommentPrefix, ruleID)

	chains := []string{"INPUT", "DOCKER-USER"}

	for _, bin := range []string{d.iptablesBin, d.ip6tablesBin} {
		for _, chain := range chains {
			d.removeFromChain(bin, chain, comment)
			d.removeFromChain(bin, chain, dropComment)
		}
	}
	return nil
}

// RemoveAllBAB 移除所有 bab 规则
func (d *DockerFirewall) RemoveAllBAB() error {
	chains := []string{"INPUT", "DOCKER-USER"}

	for _, bin := range []string{d.iptablesBin, d.ip6tablesBin} {
		for _, chain := range chains {
			d.removeAllFromChain(bin, chain, CommentPrefix)
		}
	}
	return nil
}

// GetRuleCount 获取规则数量
func (d *DockerFirewall) GetRuleCount() int {
	count := 0
	chains := []string{"INPUT", "DOCKER-USER"}

	for _, bin := range []string{d.iptablesBin, d.ip6tablesBin} {
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

// removeFromChain 从指定链中移除包含注释的规则
func (d *DockerFirewall) removeFromChain(bin, chain, comment string) {
	for {
		lineNum := d.findInChain(bin, chain, comment)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// removeAllFromChain 从指定链中移除所有包含前缀的规则
func (d *DockerFirewall) removeAllFromChain(bin, chain, prefix string) {
	for {
		lineNum := d.findInChain(bin, chain, prefix)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// findInChain 在链中查找包含指定字符串的规则行号
func (d *DockerFirewall) findInChain(bin, chain, search string) int {
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

// run 执行命令
func (d *DockerFirewall) run(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker/iptables %s 失败: %v, 输出: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

// isDockerEnvironment 检测是否为 Docker 环境
func isDockerEnvironment() bool {
	ipt := findIPTables()
	if ipt == "" {
		return false
	}
	// 检查 DOCKER-USER 链是否存在
	cmd := exec.Command(ipt, "-L", "DOCKER-USER", "-n")
	return cmd.Run() == nil
}
