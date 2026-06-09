// Package cron 实现定时任务管理，包括时间间隔解析和调度器
package cron

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/soulteary/block-area-bot/internal/config"
	"github.com/soulteary/block-area-bot/internal/repo"
	"github.com/soulteary/block-area-bot/internal/rule"
)

// ParseInterval 解析时间间隔字符串
// 支持格式: 30m, 1h, 3d, 12h, 7d
func ParseInterval(interval string) (time.Duration, error) {
	interval = strings.TrimSpace(strings.ToLower(interval))
	if interval == "" {
		return 0, fmt.Errorf("时间间隔不能为空")
	}

	// 获取数字部分和单位部分
	numStr := interval[:len(interval)-1]
	unit := interval[len(interval)-1:]

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("无效的时间间隔格式 '%s'，示例: 30m, 1h, 3d", interval)
	}

	if num <= 0 {
		return 0, fmt.Errorf("时间间隔必须大于 0")
	}

	switch unit {
	case "m":
		return time.Duration(num * float64(time.Minute)), nil
	case "h":
		return time.Duration(num * float64(time.Hour)), nil
	case "d":
		return time.Duration(num * 24 * float64(time.Hour)), nil
	default:
		return 0, fmt.Errorf("不支持的时间单位 '%s'，支持: m(分钟), h(小时), d(天)", unit)
	}
}

// Scheduler 定时调度器
type Scheduler struct {
	mu      sync.Mutex
	cfg     *config.Manager
	ruleMgr *rule.Manager
	timers  map[string]*time.Timer
	stopCh  chan struct{}
	running bool
}

// NewScheduler 创建调度器
func NewScheduler(cfg *config.Manager, ruleMgr *rule.Manager) *Scheduler {
	return &Scheduler{
		cfg:     cfg,
		ruleMgr: ruleMgr,
		timers:  make(map[string]*time.Timer),
		stopCh:  make(chan struct{}),
	}
}

// Start 启动调度器，加载所有定时任务
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	s.running = true
	cfg := s.cfg.GetConfig()

	for _, cron := range cfg.Crons {
		s.scheduleTask(cron)
	}

	log.Printf("[cron] 调度器已启动，加载了 %d 个定时任务", len(cfg.Crons))
	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopCh)

	// 停止所有定时器
	for tag, timer := range s.timers {
		timer.Stop()
		delete(s.timers, tag)
	}

	log.Printf("[cron] 调度器已停止")
}

// AddTask 添加定时任务
func (s *Scheduler) AddTask(tag, interval string) error {
	duration, err := ParseInterval(interval)
	if err != nil {
		return err
	}

	cronJob := config.CronJob{
		Tag:        tag,
		Interval:   interval,
		IntervalMs: duration.Milliseconds(),
	}

	savedCron, err := s.cfg.AddCron(cronJob)
	if err != nil {
		return err
	}

	// 如果调度器正在运行，立即调度
	s.mu.Lock()
	if s.running {
		s.scheduleTask(savedCron)
	}
	s.mu.Unlock()

	return nil
}

// RemoveTask 移除定时任务
func (s *Scheduler) RemoveTask(tag string) error {
	s.mu.Lock()
	if timer, ok := s.timers[tag]; ok {
		timer.Stop()
		delete(s.timers, tag)
	}
	s.mu.Unlock()

	return s.cfg.DelCron(tag)
}

// scheduleTask 调度单个任务（需要已持有锁或在初始化时调用）
func (s *Scheduler) scheduleTask(cron config.CronJob) {
	duration := time.Duration(cron.IntervalMs) * time.Millisecond

	// 计算下次执行时间
	var nextRun time.Duration
	if cron.LastRun.IsZero() {
		// 从未执行过，立即执行一次
		nextRun = 1 * time.Second
	} else {
		elapsed := time.Since(cron.LastRun)
		if elapsed >= duration {
			// 已超过间隔，立即执行
			nextRun = 1 * time.Second
		} else {
			nextRun = duration - elapsed
		}
	}

	timer := time.AfterFunc(nextRun, func() {
		s.executeTask(cron.Tag, duration)
	})

	// 停止旧的定时器
	if old, ok := s.timers[cron.Tag]; ok {
		old.Stop()
	}
	s.timers[cron.Tag] = timer
}

// executeTask 执行定时任务
func (s *Scheduler) executeTask(tag string, interval time.Duration) {
	log.Printf("[cron] 开始执行定时任务: %s", tag)

	// 获取数据源信息
	repoInfo, exists := s.cfg.GetRepo(tag)
	if !exists {
		log.Printf("[cron] 错误: 数据源 '%s' 不存在，跳过", tag)
		s.updateCronResult(tag, "failed", "数据源不存在")
		return
	}

	// 重新下载并解析数据
	ipData, err := repo.FetchAndParse(repoInfo.Type, repoInfo.Source)
	if err != nil {
		log.Printf("[cron] 错误: 更新数据源 '%s' 失败: %v", tag, err)
		s.updateCronResult(tag, "failed", err.Error())
		s.reschedule(tag, interval)
		return
	}

	// 保存数据
	dataPath := s.cfg.GetRepoDataPath(tag)
	if err := repo.SaveIPData(dataPath, ipData); err != nil {
		log.Printf("[cron] 错误: 保存数据源 '%s' 数据失败: %v", tag, err)
		s.updateCronResult(tag, "failed", err.Error())
		s.reschedule(tag, interval)
		return
	}

	// 更新数据源信息
	_ = s.cfg.UpdateRepo(tag, func(r *config.Repo) {
		r.IPv4Count = len(ipData.IPv4)
		r.IPv6Count = len(ipData.IPv6)
		r.UpdatedAt = time.Now()
	})

	// 刷新关联的规则（原子更新 ipset）
	if err := s.ruleMgr.RefreshRule(tag); err != nil {
		log.Printf("[cron] 警告: 刷新规则 '%s' 失败: %v", tag, err)
	}

	log.Printf("[cron] 定时任务完成: %s (IPv4: %d, IPv6: %d)",
		tag, len(ipData.IPv4), len(ipData.IPv6))
	s.updateCronResult(tag, "success", "")
	s.reschedule(tag, interval)
}

// updateCronResult 更新定时任务执行结果
func (s *Scheduler) updateCronResult(tag, result, errMsg string) {
	_ = s.cfg.UpdateCron(tag, func(c *config.CronJob) {
		c.LastRun = time.Now()
		c.LastResult = result
		c.LastError = errMsg
	})
}

// reschedule 重新调度任务
func (s *Scheduler) reschedule(tag string, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	timer := time.AfterFunc(interval, func() {
		s.executeTask(tag, interval)
	})

	if old, ok := s.timers[tag]; ok {
		old.Stop()
	}
	s.timers[tag] = timer
}

// GetNextRunTime 获取下次执行时间
func GetNextRunTime(cron config.CronJob) time.Time {
	if cron.LastRun.IsZero() {
		return time.Now()
	}
	duration := time.Duration(cron.IntervalMs) * time.Millisecond
	return cron.LastRun.Add(duration)
}
