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
// 自动兼容 Docker (DOCKER-USER 链) 和宝塔 (BT-INPUT 链) 环境
type IPTables struct {
	iptablesBin  string
	ip6tablesBin string
	// 需要操作的链列表（自动检测）
	chains []string
}

// NewIPTables 创建 IPTables 实例
// 自动检测当前环境，确定需要操作的链
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

	t := &IPTables{
		iptablesBin:  ipt,
		ip6tablesBin: ip6t,
	}

	// 自动检测需要操作的链
	t.detectChains()

	return t, nil
}

// detectChains 自动检测当前环境需要操作的链
// - INPUT 链：始终操作（主机流量）
// - DOCKER-USER 链：Docker 环境下操作（容器流量经过 FORWARD 链，不走 INPUT）
// - BT-INPUT 链：宝塔面板环境下操作（宝塔自定义链）
func (t *IPTables) detectChains() {
	t.chains = []string{"INPUT"}

	// 检测 Docker 环境：DOCKER-USER 链存在
	if t.chainExists(t.iptablesBin, "DOCKER-USER") {
		t.chains = append(t.chains, "DOCKER-USER")
	}

	// 检测宝塔环境：BT-INPUT 链存在
	if t.chainExists(t.iptablesBin, "BT-INPUT") {
		t.chains = append(t.chains, "BT-INPUT")
	}
}

// chainExists 检查指定链是否存在
func (t *IPTables) chainExists(bin, chain string) bool {
	cmd := exec.Command(bin, "-L", chain, "-n")
	return cmd.Run() == nil
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

// Name 返回后端名称（包含兼容环境信息）
func (t *IPTables) Name() string {
	extras := []string{}
	for _, chain := range t.chains {
		if chain == "DOCKER-USER" {
			extras = append(extras, "docker")
		}
		if chain == "BT-INPUT" {
			extras = append(extras, "bt")
		}
	}
	if len(extras) > 0 {
		return "iptables (兼容: " + strings.Join(extras, ", ") + ")"
	}
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
// 自动在所有检测到的链（INPUT、DOCKER-USER、BT-INPUT）中插入规则
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

	for _, chain := range t.chains {
		// IPv6 规则不操作 DOCKER-USER 链（Docker 通常不支持 IPv6 的 DOCKER-USER）
		if spec.IPv6 && chain == "DOCKER-USER" {
			continue
		}

		for _, proto := range protocols {
			args := t.buildRuleArgs(spec, proto, comment)

			if spec.Mode == "white" {
				// 白名单模式：ACCEPT 匹配的流量
				acceptArgs := append([]string{"-I", chain, "1"}, args...)
				acceptArgs = append(acceptArgs, "-j", "ACCEPT")
				if err := t.run(bin, acceptArgs...); err != nil {
					// 非 INPUT 链失败时仅跳过（链可能不完全兼容）
					if chain != "INPUT" {
						continue
					}
					return err
				}
			} else {
				// 黑名单模式：DROP 匹配的流量
				dropArgs := append([]string{"-I", chain, "1"}, args...)
				dropArgs = append(dropArgs, "-j", "DROP")
				if err := t.run(bin, dropArgs...); err != nil {
					if chain != "INPUT" {
						continue
					}
					return err
				}
			}
		}

		// 白名单模式：额外添加 DROP ALL 规则（放在最后）
		if spec.Mode == "white" {
			if spec.IPv6 && chain == "DOCKER-USER" {
				continue
			}
			dropAllComment := fmt.Sprintf("%s%d_drop", CommentPrefix, spec.RuleID)
			for _, proto := range protocols {
				var dropArgs []string
				if proto != "" {
					dropArgs = []string{"-A", chain, "-p", proto}
				} else {
					dropArgs = []string{"-A", chain}
				}
				if spec.Port != "" && proto != "" && proto != "icmp" {
					dropArgs = append(dropArgs, "--dport", normalizePort(spec.Port))
				}
				dropArgs = append(dropArgs, "-m", "comment", "--comment", dropAllComment, "-j", "DROP")
				if err := t.run(bin, dropArgs...); err != nil {
					if chain != "INPUT" {
						continue
					}
					return err
				}
			}
		}
	}

	return nil
}

// RemoveRule 移除指定规则 ID 的所有 iptables 规则
func (t *IPTables) RemoveRule(ruleID int) error {
	comment := fmt.Sprintf("%s%d", CommentPrefix, ruleID)
	dropComment := fmt.Sprintf("%s%d_drop", CommentPrefix, ruleID)

	// 从所有链中移除（iptables 和 ip6tables）
	for _, bin := range []string{t.iptablesBin, t.ip6tablesBin} {
		for _, chain := range t.chains {
			t.removeByComment(bin, chain, comment)
			t.removeByComment(bin, chain, dropComment)
		}
	}

	return nil
}

// RemoveAllBAB 移除所有 bab: 开头注释的规则
func (t *IPTables) RemoveAllBAB() error {
	for _, bin := range []string{t.iptablesBin, t.ip6tablesBin} {
		for _, chain := range t.chains {
			t.removeAllByPrefix(bin, chain, CommentPrefix)
		}
	}
	return nil
}

// GetRuleCount 获取 bab 规则数量
func (t *IPTables) GetRuleCount() int {
	count := 0
	for _, bin := range []string{t.iptablesBin, t.ip6tablesBin} {
		for _, chain := range t.chains {
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

// GetDetectedChains 返回检测到的链列表（用于状态显示）
func (t *IPTables) GetDetectedChains() []string {
	return t.chains
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
func (t *IPTables) removeByComment(bin, chain, comment string) {
	for {
		// 查找包含该注释的规则行号
		lineNum := t.findRuleInChain(bin, chain, comment)
		if lineNum == 0 {
			break
		}
		// 按行号删除
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// removeAllByPrefix 移除所有以指定前缀开头的注释的规则
func (t *IPTables) removeAllByPrefix(bin, chain, prefix string) {
	for {
		lineNum := t.findRuleInChain(bin, chain, prefix)
		if lineNum == 0 {
			break
		}
		exec.Command(bin, "-D", chain, fmt.Sprintf("%d", lineNum)).Run()
	}
}

// findRuleInChain 在指定链中查找包含指定字符串的规则行号
func (t *IPTables) findRuleInChain(bin, chain, search string) int {
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
