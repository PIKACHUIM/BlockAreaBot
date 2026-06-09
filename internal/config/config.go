// Package config 实现配置管理与数据持久化
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultConfigDir 默认配置目录
	DefaultConfigDir = "/etc/block-area-bot"
	// DefaultConfigFile 默认配置文件
	DefaultConfigFile = "/etc/block-area-bot/config.json"
	// DefaultDataDir 默认数据目录
	DefaultDataDir = "/var/lib/block-area-bot"
	// DefaultLogDir 默认日志目录
	DefaultLogDir = "/var/log/block-area-bot"
)

// Repo 数据源配置
type Repo struct {
	ID        int       `json:"id"`
	Tag       string    `json:"tag"`
	Type      string    `json:"type"`       // ipv4, ipv6, apnic:XX
	Source    string    `json:"source"`     // URL 或文件路径
	IPv4Count int       `json:"ipv4_count"` // IPv4 段数量
	IPv6Count int       `json:"ipv6_count"` // IPv6 段数量
	UpdatedAt time.Time `json:"updated_at"` // 最后更新时间
	CreatedAt time.Time `json:"created_at"` // 创建时间
}

// Rule 屏蔽规则配置
type Rule struct {
	ID        int       `json:"id"`
	Tag       string    `json:"tag"`       // 关联的数据源标签
	Mode      string    `json:"mode"`      // black 或 white
	Port      string    `json:"port"`      // 端口或端口范围，空表示全端口
	Protocols []string  `json:"protocols"` // tcp, udp, icmp，空表示全协议
	CreatedAt time.Time `json:"created_at"`
}

// CronJob 定时任务配置
type CronJob struct {
	Tag        string    `json:"tag"`         // 关联的数据源标签
	Interval   string    `json:"interval"`    // 原始间隔字符串，如 1h, 3d
	IntervalMs int64     `json:"interval_ms"` // 间隔毫秒数
	LastRun    time.Time `json:"last_run"`    // 上次执行时间
	LastResult string    `json:"last_result"` // 上次执行结果: success/failed
	LastError  string    `json:"last_error"`  // 上次错误信息
	CreatedAt  time.Time `json:"created_at"`
}

// Config 主配置结构
type Config struct {
	Repos           []Repo    `json:"repos"`
	Rules           []Rule    `json:"rules"`
	Crons           []CronJob `json:"crons"`
	NextID          int       `json:"next_id"`           // 下一个可用的 repo ID
	NextRule        int       `json:"next_rule"`         // 下一个可用的 rule ID
	FirewallBackend string    `json:"firewall_backend"`  // 防火墙后端: iptables, ufw（空则自动检测，iptables 自动兼容 Docker/宝塔环境）
}

// Manager 配置管理器
type Manager struct {
	mu         sync.RWMutex
	config     *Config
	configPath string
	dataDir    string
}

// NewManager 创建配置管理器
func NewManager() *Manager {
	return &Manager{
		configPath: DefaultConfigFile,
		dataDir:    DefaultDataDir,
	}
}

// NewManagerWithPaths 使用自定义路径创建配置管理器
func NewManagerWithPaths(configPath, dataDir string) *Manager {
	return &Manager{
		configPath: configPath,
		dataDir:    dataDir,
	}
}

// Load 加载配置文件
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，创建默认配置
			m.config = &Config{
				Repos:    []Repo{},
				Rules:    []Rule{},
				Crons:    []CronJob{},
				NextID:   1,
				NextRule: 1,
			}
			return m.saveLocked()
		}
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("解析配置文件失败（文件可能已损坏）: %w", err)
	}

	m.config = &cfg
	return nil
}

// Save 保存配置到文件
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

// saveLocked 内部保存方法（需要已持有锁）
func (m *Manager) saveLocked() error {
	// 确保配置目录存在
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 先写入临时文件，再原子替换
	tmpPath := m.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, m.configPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}

	return nil
}

// GetConfig 获取配置的只读副本
func (m *Manager) GetConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.config == nil {
		return Config{}
	}
	// 返回副本
	cfg := *m.config
	cfg.Repos = make([]Repo, len(m.config.Repos))
	copy(cfg.Repos, m.config.Repos)
	cfg.Rules = make([]Rule, len(m.config.Rules))
	copy(cfg.Rules, m.config.Rules)
	cfg.Crons = make([]CronJob, len(m.config.Crons))
	copy(cfg.Crons, m.config.Crons)
	return cfg
}

// AddRepo 添加数据源
func (m *Manager) AddRepo(repo Repo) (Repo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查标签是否重复
	for _, r := range m.config.Repos {
		if r.Tag == repo.Tag {
			return Repo{}, fmt.Errorf("数据源标签 '%s' 已存在", repo.Tag)
		}
	}

	repo.ID = m.config.NextID
	m.config.NextID++
	repo.CreatedAt = time.Now()
	m.config.Repos = append(m.config.Repos, repo)

	if err := m.saveLocked(); err != nil {
		return Repo{}, err
	}
	return repo, nil
}

// DelRepo 删除数据源（同时删除关联的规则和定时任务）
func (m *Manager) DelRepo(tagOrID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := -1
	for i, r := range m.config.Repos {
		if r.Tag == tagOrID || fmt.Sprintf("%d", r.ID) == tagOrID {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("数据源 '%s' 不存在", tagOrID)
	}

	tag := m.config.Repos[idx].Tag

	// 删除数据源
	m.config.Repos = append(m.config.Repos[:idx], m.config.Repos[idx+1:]...)

	// 级联删除关联的规则
	rules := make([]Rule, 0)
	for _, r := range m.config.Rules {
		if r.Tag != tag {
			rules = append(rules, r)
		}
	}
	m.config.Rules = rules

	// 级联删除关联的定时任务
	crons := make([]CronJob, 0)
	for _, c := range m.config.Crons {
		if c.Tag != tag {
			crons = append(crons, c)
		}
	}
	m.config.Crons = crons

	return m.saveLocked()
}

// GetRepo 获取指定数据源
func (m *Manager) GetRepo(tagOrID string) (Repo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, r := range m.config.Repos {
		if r.Tag == tagOrID || fmt.Sprintf("%d", r.ID) == tagOrID {
			return r, true
		}
	}
	return Repo{}, false
}

// UpdateRepo 更新数据源信息
func (m *Manager) UpdateRepo(tag string, updater func(*Repo)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Repos {
		if m.config.Repos[i].Tag == tag {
			updater(&m.config.Repos[i])
			return m.saveLocked()
		}
	}
	return fmt.Errorf("数据源 '%s' 不存在", tag)
}

// GenerateRepoTag 生成自动标签名
func (m *Manager) GenerateRepoTag() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := 1; ; i++ {
		tag := fmt.Sprintf("repo%d", i)
		exists := false
		for _, r := range m.config.Repos {
			if r.Tag == tag {
				exists = true
				break
			}
		}
		if !exists {
			return tag
		}
	}
}

// AddRule 添加屏蔽规则
func (m *Manager) AddRule(rule Rule) (Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 验证数据源是否存在
	found := false
	for _, r := range m.config.Repos {
		if r.Tag == rule.Tag {
			found = true
			break
		}
	}
	if !found {
		return Rule{}, fmt.Errorf("数据源 '%s' 不存在，无法创建规则", rule.Tag)
	}

	rule.ID = m.config.NextRule
	m.config.NextRule++
	rule.CreatedAt = time.Now()
	m.config.Rules = append(m.config.Rules, rule)

	if err := m.saveLocked(); err != nil {
		return Rule{}, err
	}
	return rule, nil
}

// DelRule 删除屏蔽规则
func (m *Manager) DelRule(tagOrID string, port string, protocols []string) ([]Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var deleted []Rule
	var remaining []Rule

	for _, r := range m.config.Rules {
		match := false
		if r.Tag == tagOrID || fmt.Sprintf("%d", r.ID) == tagOrID {
			// 如果指定了端口或协议，需要精确匹配
			if port != "" || len(protocols) > 0 {
				portMatch := port == "" || r.Port == port
				protoMatch := len(protocols) == 0 || protocolsEqual(r.Protocols, protocols)
				match = portMatch && protoMatch
			} else {
				match = true
			}
		}

		if match {
			deleted = append(deleted, r)
		} else {
			remaining = append(remaining, r)
		}
	}

	if len(deleted) == 0 {
		return nil, fmt.Errorf("未找到匹配的规则: %s", tagOrID)
	}

	m.config.Rules = remaining
	if err := m.saveLocked(); err != nil {
		return nil, err
	}
	return deleted, nil
}

// GetRulesByTag 获取指定标签的所有规则
func (m *Manager) GetRulesByTag(tag string) []Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var rules []Rule
	for _, r := range m.config.Rules {
		if r.Tag == tag {
			rules = append(rules, r)
		}
	}
	return rules
}

// AddCron 添加定时任务
func (m *Manager) AddCron(cron CronJob) (CronJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 验证数据源是否存在
	found := false
	for _, r := range m.config.Repos {
		if r.Tag == cron.Tag {
			found = true
			break
		}
	}
	if !found {
		return CronJob{}, fmt.Errorf("数据源 '%s' 不存在", cron.Tag)
	}

	// 检查是否已有该标签的定时任务
	for i, c := range m.config.Crons {
		if c.Tag == cron.Tag {
			// 更新已有的定时任务
			m.config.Crons[i].Interval = cron.Interval
			m.config.Crons[i].IntervalMs = cron.IntervalMs
			if err := m.saveLocked(); err != nil {
				return CronJob{}, err
			}
			return m.config.Crons[i], nil
		}
	}

	cron.CreatedAt = time.Now()
	m.config.Crons = append(m.config.Crons, cron)

	if err := m.saveLocked(); err != nil {
		return CronJob{}, err
	}
	return cron, nil
}

// DelCron 删除定时任务
func (m *Manager) DelCron(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := -1
	for i, c := range m.config.Crons {
		if c.Tag == tag {
			idx = i
			break
		}
	}

	if idx == -1 {
		return fmt.Errorf("定时任务 '%s' 不存在", tag)
	}

	m.config.Crons = append(m.config.Crons[:idx], m.config.Crons[idx+1:]...)
	return m.saveLocked()
}

// UpdateCron 更新定时任务信息
func (m *Manager) UpdateCron(tag string, updater func(*CronJob)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.config.Crons {
		if m.config.Crons[i].Tag == tag {
			updater(&m.config.Crons[i])
			return m.saveLocked()
		}
	}
	return fmt.Errorf("定时任务 '%s' 不存在", tag)
}

// EnsureDataDir 确保数据目录存在
func (m *Manager) EnsureDataDir() error {
	return os.MkdirAll(m.dataDir, 0755)
}

// GetDataDir 获取数据目录路径
func (m *Manager) GetDataDir() string {
	return m.dataDir
}

// GetRepoDataPath 获取数据源的数据文件路径
func (m *Manager) GetRepoDataPath(tag string) string {
	return filepath.Join(m.dataDir, fmt.Sprintf("%s.cidrs", tag))
}

// protocolsEqual 比较两个协议列表是否相同
func protocolsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]bool)
	for _, v := range a {
		aMap[v] = true
	}
	for _, v := range b {
		if !aMap[v] {
			return false
		}
	}
	return true
}
