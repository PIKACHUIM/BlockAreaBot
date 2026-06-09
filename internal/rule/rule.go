// Package rule 实现屏蔽规则管理的业务逻辑
package rule

import (
	"fmt"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/firewall"
	"github.com/soulteary/block-area-bot/internal/repo"
)

// Manager 规则管理器
type Manager struct {
	cfg     *config.Manager
	ipset   *firewall.IPSet
	backend firewall.Backend
}

// NewManager 创建规则管理器
func NewManager(cfg *config.Manager, ipset *firewall.IPSet, backend firewall.Backend) *Manager {
	return &Manager{
		cfg:     cfg,
		ipset:   ipset,
		backend: backend,
	}
}

// Ban 创建屏蔽规则
func (m *Manager) Ban(tag, mode, port string, protocols []string) error {
	// 验证数据源存在
	repoInfo, exists := m.cfg.GetRepo(tag)
	if !exists {
		return fmt.Errorf("数据源 '%s' 不存在", tag)
	}

	// 默认黑名单模式
	if mode == "" {
		mode = "black"
	}
	if mode != "black" && mode != "white" {
		return fmt.Errorf("无效的模式 '%s'，支持 black 或 white", mode)
	}

	// 创建规则记录
	rule := config.Rule{
		Tag:       tag,
		Mode:      mode,
		Port:      port,
		Protocols: protocols,
	}

	savedRule, err := m.cfg.AddRule(rule)
	if err != nil {
		return err
	}

	// 尝试立即应用规则（如果防火墙后端可用）
	if err := m.applyRule(savedRule, repoInfo); err != nil {
		// 应用失败不影响规则保存，记录警告
		fmt.Printf("警告: 规则已保存但应用失败（服务可能未运行）: %v\n", err)
	}

	return nil
}

// Del 删除屏蔽规则
func (m *Manager) Del(tagOrID, port string, protocols []string) ([]config.Rule, error) {
	deleted, err := m.cfg.DelRule(tagOrID, port, protocols)
	if err != nil {
		return nil, err
	}

	// 尝试移除防火墙规则
	for _, r := range deleted {
		if m.backend != nil {
			if err := m.backend.RemoveRule(r.ID); err != nil {
				fmt.Printf("警告: 移除防火墙规则 %d 失败: %v\n", r.ID, err)
			}
		}
	}

	return deleted, nil
}

// ApplyAll 应用所有已配置的规则（服务启动时调用）
func (m *Manager) ApplyAll() error {
	cfg := m.cfg.GetConfig()

	for _, rule := range cfg.Rules {
		repoInfo, exists := m.cfg.GetRepo(rule.Tag)
		if !exists {
			fmt.Printf("警告: 规则 %d 引用的数据源 '%s' 不存在，跳过\n", rule.ID, rule.Tag)
			continue
		}

		if err := m.applyRule(rule, repoInfo); err != nil {
			fmt.Printf("警告: 应用规则 %d 失败: %v\n", rule.ID, err)
		}
	}

	return nil
}

// RemoveAll 移除所有防火墙规则（服务停止时调用）
func (m *Manager) RemoveAll() error {
	if m.backend == nil {
		return nil
	}
	return m.backend.RemoveAllBAB()
}

// applyRule 应用单条规则
func (m *Manager) applyRule(rule config.Rule, repoInfo config.Repo) error {
	if m.backend == nil {
		return fmt.Errorf("防火墙后端未初始化")
	}

	// 加载 IP 数据
	dataPath := m.cfg.GetRepoDataPath(rule.Tag)
	ipData, err := repo.LoadIPData(dataPath)
	if err != nil {
		return fmt.Errorf("加载数据源 '%s' 的数据失败: %w", rule.Tag, err)
	}

	// 创建并填充 ipset 集合（IPv4）
	if len(ipData.IPv4) > 0 {
		if err := m.ipset.CreateAndPopulate(rule.Tag, false, ipData.IPv4); err != nil {
			return fmt.Errorf("创建 IPv4 ipset 集合失败: %w", err)
		}

		// 应用防火墙规则
		spec := firewall.RuleSpec{
			RuleID:    rule.ID,
			SetName:   firewall.SetName(rule.Tag, false, 0),
			Mode:      rule.Mode,
			Port:      rule.Port,
			Protocols: rule.Protocols,
			IPv6:      false,
		}
		if err := m.backend.ApplyRule(spec); err != nil {
			return fmt.Errorf("应用 IPv4 防火墙规则失败: %w", err)
		}
	}

	// 创建并填充 ipset 集合（IPv6）
	if len(ipData.IPv6) > 0 {
		if err := m.ipset.CreateAndPopulate(rule.Tag, true, ipData.IPv6); err != nil {
			return fmt.Errorf("创建 IPv6 ipset 集合失败: %w", err)
		}

		// 应用防火墙规则
		spec := firewall.RuleSpec{
			RuleID:    rule.ID,
			SetName:   firewall.SetName(rule.Tag, true, 0),
			Mode:      rule.Mode,
			Port:      rule.Port,
			Protocols: rule.Protocols,
			IPv6:      true,
		}
		if err := m.backend.ApplyRule(spec); err != nil {
			return fmt.Errorf("应用 IPv6 防火墙规则失败: %w", err)
		}
	}

	_ = repoInfo
	return nil
}

// RefreshRule 刷新指定数据源关联的所有规则（数据源更新后调用）
func (m *Manager) RefreshRule(tag string) error {
	rules := m.cfg.GetRulesByTag(tag)
	if len(rules) == 0 {
		return nil
	}

	_, exists := m.cfg.GetRepo(tag)
	if !exists {
		return fmt.Errorf("数据源 '%s' 不存在", tag)
	}

	// 加载新数据
	dataPath := m.cfg.GetRepoDataPath(tag)
	ipData, err := repo.LoadIPData(dataPath)
	if err != nil {
		return fmt.Errorf("加载数据源 '%s' 的数据失败: %w", tag, err)
	}

	// 原子更新 ipset 集合
	if len(ipData.IPv4) > 0 {
		if err := m.ipset.AtomicUpdate(tag, false, ipData.IPv4); err != nil {
			return fmt.Errorf("原子更新 IPv4 ipset 失败: %w", err)
		}
	}
	if len(ipData.IPv6) > 0 {
		if err := m.ipset.AtomicUpdate(tag, true, ipData.IPv6); err != nil {
			return fmt.Errorf("原子更新 IPv6 ipset 失败: %w", err)
		}
	}

	// 防火墙规则不需要更新，因为它们引用的是 ipset 集合名称
	return nil
}