package cmd

import (
	"fmt"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/cron"
	"github.com/soulteary/block-area-bot/internal/firewall"
	"github.com/soulteary/block-area-bot/internal/rule"
)

// cronAdd 添加定时任务
func cronAdd(tag, interval string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	// 验证间隔格式
	duration, err := cron.ParseInterval(interval)
	if err != nil {
		return err
	}

	ipset, _ := firewall.NewIPSet()
	backend := initBackend(cfg)
	ruleMgr := rule.NewManager(cfg, ipset, backend)
	scheduler := cron.NewScheduler(cfg, ruleMgr)

	if err := scheduler.AddTask(tag, interval); err != nil {
		return err
	}

	fmt.Printf("✓ 定时任务添加成功\n")
	fmt.Printf("  数据源: %s\n", tag)
	fmt.Printf("  间隔:   %s (%v)\n", interval, duration)

	return nil
}

// cronDel 删除定时任务
func cronDel(tag string) error {
	if err := checkRoot(); err != nil {
		return err
	}

	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	ipset, _ := firewall.NewIPSet()
	backend := initBackend(cfg)
	ruleMgr := rule.NewManager(cfg, ipset, backend)
	scheduler := cron.NewScheduler(cfg, ruleMgr)

	if err := scheduler.RemoveTask(tag); err != nil {
		return err
	}

	fmt.Printf("✓ 定时任务 '%s' 已删除\n", tag)
	return nil
}

// cronList 列出所有定时任务
func cronList() error {
	cfg := config.NewManager()
	if err := cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}

	c := cfg.GetConfig()
	if len(c.Crons) == 0 {
		fmt.Println("暂无定时任务，使用 'block cron add' 添加")
		return nil
	}

	fmt.Println("定时任务列表:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  %-12s %-8s %-20s %-20s %s\n",
		"标签", "间隔", "上次执行", "下次执行", "结果")
	fmt.Println("  ────────────────────────────────────────────────────────────")

	for _, cr := range c.Crons {
		lastRun := "从未执行"
		if !cr.LastRun.IsZero() {
			lastRun = cr.LastRun.Format("2006-01-02 15:04:05")
		}

		nextRun := cron.GetNextRunTime(cr).Format("2006-01-02 15:04:05")

		result := cr.LastResult
		if result == "" {
			result = "-"
		}
		if cr.LastError != "" {
			result += " (" + cr.LastError + ")"
		}

		fmt.Printf("  %-12s %-8s %-20s %-20s %s\n",
			cr.Tag, cr.Interval, lastRun, nextRun, result)
	}

	return nil
}