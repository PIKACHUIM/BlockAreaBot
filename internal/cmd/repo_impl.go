package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/repo"
)

// repoAdd 添加数据源
func repoAdd(repoType, tag, source string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	if repoType == "" {
		return fmt.Errorf("必须指定数据源类型 (--type)")
	}

	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if err := cfg.EnsureDataDir(); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	// 自动生成标签
	if tag == "" {
		tag = cfg.GenerateRepoTag()
	}

	// 验证类型
	validType := repoType == "ipv4" || repoType == "ipv6" || strings.HasPrefix(repoType, "apnic:")
	if !validType {
		return fmt.Errorf("无效的类型 '%s'，支持: ipv4, ipv6, apnic:XX", repoType)
	}

	// 下载并解析数据
	fmt.Printf("正在获取数据源 [%s] (类型: %s)...\n", tag, repoType)
	ipData, err := repo.FetchAndParse(repoType, source)
	if err != nil {
		return fmt.Errorf("获取数据失败: %w", err)
	}

	// 保存数据文件
	dataPath := cfg.GetRepoDataPath(tag)
	if err := repo.SaveIPData(dataPath, ipData); err != nil {
		return fmt.Errorf("保存数据失败: %w", err)
	}

	// 确定实际的 source
	actualSource := source
	if actualSource == "" && strings.HasPrefix(repoType, "apnic:") {
		actualSource = repo.ApnicURL
	}

	// 保存配置
	repoConfig := config.Repo{
		Tag:       tag,
		Type:      repoType,
		Source:    actualSource,
		IPv4Count: len(ipData.IPv4),
		IPv6Count: len(ipData.IPv6),
		UpdatedAt: time.Now(),
	}

	savedRepo, err := cfg.AddRepo(repoConfig)
	if err != nil {
		return err
	}

	fmt.Printf("✓ 数据源添加成功\n")
	fmt.Printf("  ID:    %d\n", savedRepo.ID)
	fmt.Printf("  标签:  %s\n", savedRepo.Tag)
	fmt.Printf("  类型:  %s\n", savedRepo.Type)
	fmt.Printf("  来源:  %s\n", savedRepo.Source)
	fmt.Printf("  IPv4:  %d 条\n", savedRepo.IPv4Count)
	fmt.Printf("  IPv6:  %d 条\n", savedRepo.IPv6Count)

	return nil
}

// repoDel 删除数据源
func repoDel(tagOrID string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	if err := cfg.DelRepo(tagOrID); err != nil {
		return err
	}

	fmt.Printf("✓ 数据源 '%s' 已删除（关联的规则和定时任务也已移除）\n", tagOrID)
	return nil
}

// repoList 列出所有数据源
func repoList() error {
	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	c := cfg.GetConfig()
	if len(c.Repos) == 0 {
		fmt.Println("暂无数据源，使用 'block repo add' 添加")
		return nil
	}

	fmt.Println("数据源列表:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  %-4s %-12s %-12s %-40s %-8s %-8s %s\n",
		"ID", "标签", "类型", "来源", "IPv4", "IPv6", "更新时间")
	fmt.Println("  ────────────────────────────────────────────────────────────")

	for _, r := range c.Repos {
		source := r.Source
		if len(source) > 38 {
			source = source[:35] + "..."
		}
		updatedAt := "-"
		if !r.UpdatedAt.IsZero() {
			updatedAt = r.UpdatedAt.Format("01-02 15:04")
		}
		fmt.Printf("  %-4d %-12s %-12s %-40s %-8d %-8d %s\n",
			r.ID, r.Tag, r.Type, source, r.IPv4Count, r.IPv6Count, updatedAt)
	}

	return nil
}