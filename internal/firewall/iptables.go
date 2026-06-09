package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	// CommentPrefix iptables 规则注释前缀
	CommentPrefix = "bab:"
)

// IPTables 封装 iptables/ip6tables 命令操作
type IPTables struct {
	iptablesBin  string
	ip6tablesBin string
}

// NewIPTables 创建 IPTables 实例
func NewIPTables() (*IPTables, error) {
	// 优先查找 iptables-legacy，然后 iptables
	ipt := findIPTables()
	if ipt == "" {
		return nil, fmt.Errorf("未找到 iptables 命令，请确保已安装 iptables")
	}

	ip6t := findIP6Tables()
	if ip6t == "" {
		return nil, fmt.Errorf("未找到 ip6tables 命令，请确保已安装 iptables")
	}

	return &IPTables{
		iptablesBin:  ipt,
		ip6tablesBin: ip6t,
	}, nil
}

// findIPTables 查找 iptables 可执行文件
func findIPTables() string {
	// 优先使用 iptables-legacy
	if bin, err := exec.LookPath("iptables-legacy"); err == nil {
		return bin
	}
	if bin, err := exec.LookPath("iptables"); err == nil {
		return bin
	}
	return ""
}

// findIP6Tables 查找 ip6tables 可执行文件
func findIP6Tables() string {
	if bin, err := exec.LookPath("ip6tables-legacy"); err == nil {
		return bin
	}
	if bin, err := exec.LookPath("ip6tables"); err == nil {
		return bin
	}
	return ""
}

// Name 返回后端名称
func (t *IPTables) Name() string {
	return "iptables"
}

// CheckAvailable 检查 iptables 是否可用
func (t *IPTables) CheckAvailable() error {
	cmd := exec.Command(t.iptablesBin, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("iptables 不可用: %v, 输出: %s", err, string(output))
	}
	return nil
}

// RuleSpec 规则规格
type RuleSpec struct {
	RuleID    int      // 规则 ID（用于注释标记）
	SetName   string   // ipset 集合名称
	Mode      string   // black 或 white
	Port      string   // 端口或端口范围
	Protocols []string // 协议列表
	IPv6      bool     // 是否为 IPv6 规则
}

// ApplyRule 应用一条规则到 iptables
func (t *IPTables) ApplyRule(spec RuleSpec) error {
	bin := t.iptablesBin
	if spec.IPv6 {
		bin = t.ip6tablesBin
	}

	comment := fmt.Sprintf("%s%d", CommentPrefix, spec.RuleID)

	// 确定协议列表
	protocols := spec.Protocols
	if len(protocols) == 0 {
		// 无协议限制时，根据是否有端口来决定
		if spec.Port != "" {
			// 有端口时需要指定协议
			protocols = []string{"tcp", "udp"}
		} else {
			protocols = []string{""} // 空字符串表示不限协议
		}
	}

	for _, proto := range protocols {
		args := t.buildRuleArgs(spec, proto, comment)

		if spec.Mode == "white" {
			// 白名单模式：ACCEPT 匹配的流量
			acceptArgs := append([]string{"-A", "INPUT"}, args...)
			acceptArgs = append(acceptArgs, "-j", "ACCEPT")
			if err := t.run(bin, acceptArgs...); err != nil {
				return err
			}
		} else {
			// 黑名单模式：DROP 匹配的流量
			dropArgs := append([]string{"-A", "INPUT"}, args...)
			dropArgs = append(dropArgs, "-j", "DROP")
			if err := t.run(bin, dropArgs...); err != nil {
				return err
			}
		}
	}

	// 白名单模式：额外添加 DROP ALL 规则（放在最后）
	if spec.Mode == "white" {
		dropAllComment := fmt.Sprintf("%s%d_drop", CommentPrefix, spec.RuleID)
		for _, proto := range protocols {
			var dropArgs []string
			if proto != "" {
				dropArgs = []string{"-A", "INPUT", "-p", proto}
			} else {
				dropArgs = []string{"-A", "INPUT"}
			}
			if spec.Port != "" && proto != "" {
				dropArgs = append(dropArgs, "--dport", normalizePort(spec.Port))
			}
			dropArgs = append(dropArgs, "-m", "comment", "--comment", dropAllComment, "-j", "DROP")
			if err := t.run(bin, dropArgs...); err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveRule 移除指定规则 ID 的所有 iptables 规则
func (t *IPTables) RemoveRule(ruleID int) error {
	comment := fmt.Sprintf("%s%d", CommentPrefix, ruleID)
	dropComment := fmt.Sprintf("%s%d_drop", CommentPrefix, ruleID)

	// 从 iptables 和 ip6tables 中移除
	for _, bin := range []string{t.iptablesBin, t.ip6tablesBin} {
		if err := t.removeByComment(bin, comment); err != nil {
			return err
		}
		if err := t.removeByComment(bin, dropComment); err != nil {
			return err
		}
	}

	return nil
}

// RemoveAllBAB 移除所有 bab: 开头注释的规则
func (t *IPTables) RemoveAllBAB() error {
	for _, bin := range []string{t.iptablesBin, t.ip6tablesBin} {
		if err := t.removeAllByPrefix(bin, CommentPrefix); err != nil {
			return err
		}
	}
	return nil
}

// GetRuleCount 获取 bab 规则数量
func (t *IPTables) GetRuleCount() int {
	count := 0
	for _, bin := range []string{t.iptablesBin, t.ip6tablesBin} {
		cmd := exec.Command(bin, "-L", "INPUT", "-n", "--line-numbers")
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

// buildRuleArgs 构建规则参数
func (t *IPTables) buildRuleArgs(spec RuleSpec, proto, comment string) []string {
	var args []string

	// 协议
	if proto != "" {
		args = append(args, "-p", proto)
	}

	// 端口
	if spec.Port != "" && proto != "" && proto != "icmp" {
		args = append(args, "--dport", normalizePort(spec.Port))
	}

	// ipset 匹配
	args = append(args, "-m", "set", "--match-set", spec.SetName, "src")

	// 注释
	args = append(args, "-m", "comment", "--comment", comment)

	return args
}

// removeByComment 移除包含指定注释的所有规则
func (t *IPTables) removeByComment(bin, comment string) error {
	for {
		// 查找包含该注释的规则行号
		lineNum := t.findRuleByComment(bin, comment)
		if lineNum == 0 {
			break
		}

		// 按行号删除
		if err := t.run(bin, "-D", "INPUT", fmt.Sprintf("%d", lineNum)); err != nil {
			return err
		}
	}
	return nil
}

// removeAllByPrefix 移除所有以指定前缀开头的注释的规则
func (t *IPTables) removeAllByPrefix(bin, prefix string) error {
	for {
		lineNum := t.findRuleByCommentPrefix(bin, prefix)
		if lineNum == 0 {
			break
		}

		if err := t.run(bin, "-D", "INPUT", fmt.Sprintf("%d", lineNum)); err != nil {
			return err
		}
	}
	return nil
}

// findRuleByComment 查找包含指定注释的规则行号
func (t *IPTables) findRuleByComment(bin, comment string) int {
	cmd := exec.Command(bin, "-L", "INPUT", "-n", "--line-numbers")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, comment) {
			var num int
			fmt.Sscanf(line, "%d", &num)
			if num > 0 {
				return num
			}
		}
	}
	return 0
}

// findRuleByCommentPrefix 查找包含指定注释前缀的规则行号
func (t *IPTables) findRuleByCommentPrefix(bin, prefix string) int {
	cmd := exec.Command(bin, "-L", "INPUT", "-n", "--line-numbers")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, prefix) {
			var num int
			fmt.Sscanf(line, "%d", &num)
			if num > 0 {
				return num
			}
		}
	}
	return 0
}

// normalizePort 将端口范围格式从 10000-19999 转换为 iptables 格式 10000:19999
func normalizePort(port string) string {
	return strings.ReplaceAll(port, "-", ":")
}

// run 执行 iptables 命令
func (t *IPTables) run(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s 失败: %v, 输出: %s", bin, strings.Join(args, " "), err, string(output))
	}
	return nil
}
