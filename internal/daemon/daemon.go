// Package daemon 实现服务守护进程
package daemon

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/cron"
	"github.com/soulteary/block-area-bot/internal/firewall"
	"github.com/soulteary/block-area-bot/internal/logger"
	"github.com/soulteary/block-area-bot/internal/rule"
)

// Daemon 守护进程
type Daemon struct {
	cfg       *config.Manager
	ipset     *firewall.IPSet
	backend   firewall.Backend
	ruleMgr   *rule.Manager
	scheduler *cron.Scheduler
}

// New 创建守护进程实例
func New() (*Daemon, error) {
	return &Daemon{}, nil
}

// Run 运行守护进程
func (d *Daemon) Run() error {
	// 初始化日志（daemon 模式）
	if err := logger.Init(true); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
	}
	defer logger.Close()

	logger.Info("[daemon] Block Area Bot 守护进程启动中...")

	// 1. 加载配置
	d.cfg = config.NewManager()
	if err := d.cfg.Load(); err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	if err := d.cfg.EnsureDataDir(); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}
	logger.Info("[daemon] 配置加载完成")

	// 2. 检测 ipset 依赖
	ipset, err := firewall.NewIPSet()
	if err != nil {
		return fmt.Errorf("初始化 ipset 失败: %w", err)
	}
	if err := ipset.CheckAvailable(); err != nil {
		return err
	}
	d.ipset = ipset
	logger.Info("[daemon] ipset 可用")

	// 3. 自动检测并初始化防火墙后端
	cfg := d.cfg.GetConfig()
	backendType := firewall.BackendType(cfg.FirewallBackend)
	if backendType == "" {
		// 自动检测
		backendType = firewall.DetectBackend()
	}

	backend, err := firewall.NewBackend(backendType)
	if err != nil {
		return fmt.Errorf("初始化防火墙后端 '%s' 失败: %w", backendType, err)
	}
	if err := backend.CheckAvailable(); err != nil {
		return fmt.Errorf("防火墙后端 '%s' 不可用: %w", backendType, err)
	}
	d.backend = backend
	logger.Info("[daemon] 防火墙后端: %s", backend.Name())

	// 4. 创建规则管理器
	d.ruleMgr = rule.NewManager(d.cfg, d.ipset, d.backend)

	// 5. 应用所有规则
	if err := d.ruleMgr.ApplyAll(); err != nil {
		logger.Warn("[daemon] 应用规则时出现错误: %v", err)
	}
	logger.Info("[daemon] 规则应用完成")

	// 6. 启动定时调度器
	d.scheduler = cron.NewScheduler(d.cfg, d.ruleMgr)
	if err := d.scheduler.Start(); err != nil {
		logger.Warn("[daemon] 启动调度器失败: %v", err)
	}
	logger.Info("[daemon] 定时调度器已启动")

	logger.Info("[daemon] Block Area Bot 守护进程已就绪")

	// 7. 等待信号
	d.waitForSignal()

	return nil
}

// waitForSignal 等待系统信号
func (d *Daemon) waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigCh
	logger.Info("[daemon] 收到信号 %v，开始优雅停止...", sig)

	d.shutdown()
}

// shutdown 优雅停止
func (d *Daemon) shutdown() {
	// 1. 停止调度器
	if d.scheduler != nil {
		d.scheduler.Stop()
		logger.Info("[daemon] 调度器已停止")
	}

	// 2. 清除所有防火墙规则
	if d.ruleMgr != nil {
		if err := d.ruleMgr.RemoveAll(); err != nil {
			logger.Warn("[daemon] 清除防火墙规则失败: %v", err)
		} else {
			logger.Info("[daemon] 防火墙规则已清除")
		}
	}

	// 3. 销毁所有 ipset 集合
	if d.ipset != nil {
		if err := d.ipset.DestroyAllBAB(); err != nil {
			logger.Warn("[daemon] 销毁 ipset 集合失败: %v", err)
		} else {
			logger.Info("[daemon] ipset 集合已销毁")
		}
	}

	logger.Info("[daemon] Block Area Bot 守护进程已停止")
}