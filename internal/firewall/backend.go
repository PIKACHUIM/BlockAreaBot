// Package firewall 定义防火墙后端接口和多后端支持
package firewall

// Backend 防火墙后端接口
// 所有防火墙后端（iptables、ufw）都需要实现此接口
// iptables 后端会自动兼容 Docker (DOCKER-USER 链) 和宝塔 (BT-INPUT 链) 环境
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
)

// DetectBackend 自动检测可用的防火墙后端
// 优先检测 ufw（如果已启用），否则使用 iptables
// iptables 后端会自动兼容 Docker 和宝塔环境的链结构
func DetectBackend() BackendType {
	// 检测 ufw 是否已启用
	if isUFWAvailable() {
		return BackendUFW
	}
	// 默认使用 iptables（自动兼容 Docker/宝塔）
	return BackendIPTables
}

// NewBackend 根据类型创建防火墙后端
func NewBackend(backendType BackendType) (Backend, error) {
	switch backendType {
	case BackendUFW:
		return NewUFW()
	default:
		return NewIPTables()
	}
}