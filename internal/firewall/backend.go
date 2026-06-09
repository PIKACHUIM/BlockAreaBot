// Package firewall 定义防火墙后端接口和多后端支持
package firewall

// Backend 防火墙后端接口
// 所有防火墙后端（iptables、ufw、docker、宝塔）都需要实现此接口
type Backend interface {
	// Name 返回后端名称
	Name() string

	// CheckAvailable 检查后端是否可用
	CheckAvailable() error

	// ApplyRule 应用一条屏蔽规则
	ApplyRule(spec RuleSpec) error

	// RemoveRule 移除指定规则 ID 的所有规则
	RemoveRule(ruleID int) error

	// RemoveAllBAB 移除所有由本程序创建的规则
	RemoveAllBAB() error

	// GetRuleCount 获取本程序创建的规则数量
	GetRuleCount() int
}

// BackendType 防火墙后端类型
type BackendType string

const (
	BackendIPTables BackendType = "iptables"
	BackendUFW      BackendType = "ufw"
	BackendDocker   BackendType = "docker"
	BackendBT       BackendType = "bt" // 宝塔
)

// DetectBackend 自动检测可用的防火墙后端
func DetectBackend() BackendType {
	// 优先检测宝塔（宝塔底层也用 iptables，但有自己的管理方式）
	if isBTPanel() {
		return BackendBT
	}
	// 检测 ufw
	if isUFWAvailable() {
		return BackendUFW
	}
	// 检测 Docker 环境（DOCKER chain 存在）
	if isDockerEnvironment() {
		return BackendDocker
	}
	// 默认使用 iptables
	return BackendIPTables
}

// NewBackend 根据类型创建防火墙后端
func NewBackend(backendType BackendType) (Backend, error) {
	switch backendType {
	case BackendUFW:
		return NewUFW()
	case BackendDocker:
		return NewDockerFirewall()
	case BackendBT:
		return NewBTFirewall()
	default:
		return NewIPTables()
	}
}
