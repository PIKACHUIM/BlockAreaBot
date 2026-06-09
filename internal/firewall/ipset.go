// Package firewall 封装 ipset 和 iptables 操作
package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	// MaxElemDefault ipset 集合默认最大元素数
	MaxElemDefault = 131072
	// SetPrefix ipset 集合名称前缀
	SetPrefix = "bab_"
)

// IPSet 封装 ipset 命令操作
type IPSet struct {
	ipsetBin string
}

// NewIPSet 创建 IPSet 实例
func NewIPSet() (*IPSet, error) {
	bin, err := exec.LookPath("ipset")
	if err != nil {
		return nil, fmt.Errorf("未找到 ipset 命令，请确保已安装 ipset: %w", err)
	}
	return &IPSet{ipsetBin: bin}, nil
}

// CheckAvailable 检查 ipset 是否可用
func (s *IPSet) CheckAvailable() error {
	cmd := exec.Command(s.ipsetBin, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ipset 不可用: %v, 输出: %s", err, string(output))
	}
	return nil
}

// SetName 生成 ipset 集合名称
// tag: 数据源标签, ipv6: 是否为 IPv6 集合, index: 分片索引（0表示不分片）
func SetName(tag string, ipv6 bool, index int) string {
	name := SetPrefix + tag
	if ipv6 {
		name += "_v6"
	}
	if index > 0 {
		name += fmt.Sprintf("_%d", index)
	}
	// ipset 名称最长 31 字符
	if len(name) > 31 {
		name = name[:31]
	}
	return name
}

// TempSetName 生成临时集合名称（用于原子替换）
func TempSetName(tag string, ipv6 bool, index int) string {
	name := SetPrefix + "tmp_" + tag
	if ipv6 {
		name += "_v6"
	}
	if index > 0 {
		name += fmt.Sprintf("_%d", index)
	}
	if len(name) > 31 {
		name = name[:31]
	}
	return name
}

// Create 创建 ipset 集合
func (s *IPSet) Create(name string, ipv6 bool, maxElem int) error {
	family := "inet"
	if ipv6 {
		family = "inet6"
	}
	if maxElem <= 0 {
		maxElem = MaxElemDefault
	}

	args := []string{"create", name, "hash:net", "family", family, "maxelem", fmt.Sprintf("%d", maxElem), "-exist"}
	return s.run(args...)
}

// Destroy 销毁 ipset 集合
func (s *IPSet) Destroy(name string) error {
	return s.run("destroy", name)
}

// DestroyIfExists 如果存在则销毁集合
func (s *IPSet) DestroyIfExists(name string) error {
	if s.Exists(name) {
		return s.Destroy(name)
	}
	return nil
}

// Flush 清空 ipset 集合
func (s *IPSet) Flush(name string) error {
	return s.run("flush", name)
}

// Add 向集合添加 CIDR
func (s *IPSet) Add(name, cidr string) error {
	return s.run("add", name, cidr, "-exist")
}

// AddBatch 批量添加 CIDR 到集合（使用 restore 命令提高效率）
func (s *IPSet) AddBatch(name string, cidrs []string) error {
	if len(cidrs) == 0 {
		return nil
	}

	// 构建 restore 输入
	var sb strings.Builder
	for _, cidr := range cidrs {
		sb.WriteString(fmt.Sprintf("add %s %s -exist\n", name, cidr))
	}

	cmd := exec.Command(s.ipsetBin, "restore")
	cmd.Stdin = strings.NewReader(sb.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ipset restore 失败: %v, 输出: %s", err, string(output))
	}
	return nil
}

// Swap 原子交换两个集合
func (s *IPSet) Swap(set1, set2 string) error {
	return s.run("swap", set1, set2)
}

// Exists 检查集合是否存在
func (s *IPSet) Exists(name string) bool {
	cmd := exec.Command(s.ipsetBin, "list", name, "-name")
	err := cmd.Run()
	return err == nil
}

// ListSets 列出所有以 bab_ 开头的集合
func (s *IPSet) ListSets() ([]string, error) {
	cmd := exec.Command(s.ipsetBin, "list", "-name")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("列出 ipset 集合失败: %w", err)
	}

	var sets []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, SetPrefix) {
			sets = append(sets, line)
		}
	}
	return sets, nil
}

// CountEntries 获取集合中的条目数量
func (s *IPSet) CountEntries(name string) (int, error) {
	cmd := exec.Command(s.ipsetBin, "list", name, "-terse")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("获取集合信息失败: %w", err)
	}

	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Number of entries:") {
			var count int
			fmt.Sscanf(line, "Number of entries: %d", &count)
			return count, nil
		}
	}
	return 0, nil
}

// AtomicUpdate 原子更新集合内容
// 创建临时集合 → 填充数据 → swap 替换 → 删除旧临时集合
func (s *IPSet) AtomicUpdate(tag string, ipv6 bool, cidrs []string) error {
	if len(cidrs) == 0 {
		return nil
	}

	// 计算需要多少个分片
	maxElem := MaxElemDefault
	numSets := (len(cidrs) + maxElem - 1) / maxElem

	for i := 0; i < numSets; i++ {
		setName := SetName(tag, ipv6, i)
		tmpName := TempSetName(tag, ipv6, i)

		// 计算当前分片的 CIDR 范围
		start := i * maxElem
		end := start + maxElem
		if end > len(cidrs) {
			end = len(cidrs)
		}
		chunk := cidrs[start:end]

		// 创建临时集合
		if err := s.Create(tmpName, ipv6, maxElem); err != nil {
			return fmt.Errorf("创建临时集合 %s 失败: %w", tmpName, err)
		}

		// 填充数据
		if err := s.AddBatch(tmpName, chunk); err != nil {
			s.DestroyIfExists(tmpName)
			return fmt.Errorf("填充临时集合 %s 失败: %w", tmpName, err)
		}

		// 确保目标集合存在
		if !s.Exists(setName) {
			if err := s.Create(setName, ipv6, maxElem); err != nil {
				s.DestroyIfExists(tmpName)
				return fmt.Errorf("创建目标集合 %s 失败: %w", setName, err)
			}
		}

		// 原子交换
		if err := s.Swap(setName, tmpName); err != nil {
			s.DestroyIfExists(tmpName)
			return fmt.Errorf("交换集合 %s <-> %s 失败: %w", setName, tmpName, err)
		}

		// 删除旧的临时集合
		s.DestroyIfExists(tmpName)
	}

	return nil
}

// CreateAndPopulate 创建集合并填充数据（首次创建时使用）
func (s *IPSet) CreateAndPopulate(tag string, ipv6 bool, cidrs []string) error {
	if len(cidrs) == 0 {
		return nil
	}

	maxElem := MaxElemDefault
	numSets := (len(cidrs) + maxElem - 1) / maxElem

	for i := 0; i < numSets; i++ {
		setName := SetName(tag, ipv6, i)

		start := i * maxElem
		end := start + maxElem
		if end > len(cidrs) {
			end = len(cidrs)
		}
		chunk := cidrs[start:end]

		// 创建集合
		if err := s.Create(setName, ipv6, maxElem); err != nil {
			return fmt.Errorf("创建集合 %s 失败: %w", setName, err)
		}

		// 填充数据
		if err := s.AddBatch(setName, chunk); err != nil {
			return fmt.Errorf("填充集合 %s 失败: %w", setName, err)
		}
	}

	return nil
}

// DestroyAll 销毁指定标签的所有集合
func (s *IPSet) DestroyAll(tag string) error {
	sets, err := s.ListSets()
	if err != nil {
		return err
	}

	prefix := SetPrefix + tag
	for _, set := range sets {
		if strings.HasPrefix(set, prefix) {
			if err := s.Destroy(set); err != nil {
				return fmt.Errorf("销毁集合 %s 失败: %w", set, err)
			}
		}
	}
	return nil
}

// DestroyAllBAB 销毁所有 bab_ 开头的集合
func (s *IPSet) DestroyAllBAB() error {
	sets, err := s.ListSets()
	if err != nil {
		return err
	}

	for _, set := range sets {
		if err := s.Destroy(set); err != nil {
			return fmt.Errorf("销毁集合 %s 失败: %w", set, err)
		}
	}
	return nil
}

// GetSetCount 获取 bab_ 集合数量
func (s *IPSet) GetSetCount() int {
	sets, err := s.ListSets()
	if err != nil {
		return 0
	}
	return len(sets)
}

// run 执行 ipset 命令
func (s *IPSet) run(args ...string) error {
	cmd := exec.Command(s.ipsetBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ipset %s 失败: %v, 输出: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}
