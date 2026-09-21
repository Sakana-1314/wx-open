package service

import (
	"context"
	"log"
	"time"

	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// Scheduler 定时对账与清理。
//
// 职责（都为「兜底」性质，主链路仍由回调与作业引擎驱动）：
//   - 审核状态对账：审核结果虽然会推送事件，但推送可能丢失，因此定期用 get_latest_auditstatus 核对；
//   - 授权方信息同步：商家改昵称/头像/类目后，平台侧需要定期刷新快照；
//   - 票据新鲜度检查：超过 20 分钟未收到 component_verify_ticket 说明推送可能已中断（官方周期是 10 分钟），
//     在日志中告警并在概览页红标，必要时可用 api_start_push_ticket 恢复；
//   - 日志清理：按设置页的保留天数清理调用日志与回调事件。
//
// 与作业引擎一样，调度器也依赖数据库排他锁保证多实例下只有一个在跑。
type Scheduler struct {
	container *Container
	interval  time.Duration

	lastAuditSync      time.Time
	lastAuthorizerSync time.Time
	lastTicketCheck    time.Time
	lastPrune          time.Time
	locked             bool
}

// 各类任务的执行周期。
const (
	auditSyncInterval      = 5 * time.Minute
	authorizerSyncInterval = 60 * time.Minute
	ticketCheckInterval    = 5 * time.Minute
	pruneInterval          = 24 * time.Hour
)

// NewScheduler 构造调度器。
func NewScheduler(container *Container) *Scheduler {
	return &Scheduler{container: container, interval: time.Minute}
}

// Start 启动调度循环（阻塞在后台 goroutine 中，随 ctx 结束）。
func (s *Scheduler) Start(ctx context.Context) {
	// 拿不到锁说明另一个实例在跑调度，跳过即可（不是错误）。
	lockName := "wx_platform_scheduler:" + s.container.Env.Cfg.DBName
	ok, err := s.container.Env.Repos.Jobs.TryLock(ctx, lockName)
	if err != nil {
		log.Printf("[scheduler] 获取调度锁失败，定时对账已禁用: %v", err)
		return
	}
	if !ok {
		log.Printf("[scheduler] 另一个实例已持有调度锁，本实例不执行定时对账")
		return
	}
	s.locked = true

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now()

	if s.container.Audit != nil && now.Sub(s.lastAuditSync) >= auditSyncInterval {
		s.lastAuditSync = now
		s.syncAudits(ctx)
	}
	if s.container.Authorizer != nil && now.Sub(s.lastAuthorizerSync) >= authorizerSyncInterval {
		s.lastAuthorizerSync = now
		s.syncAuthorizers(ctx)
	}
	if now.Sub(s.lastTicketCheck) >= ticketCheckInterval {
		s.lastTicketCheck = now
		s.checkTicket(ctx)
	}
	if now.Sub(s.lastPrune) >= pruneInterval {
		s.lastPrune = now
		s.pruneLogs(ctx)
	}
}

// syncAudits 只对「审核中」的小程序对账，避免无意义地打满调用量。
func (s *Scheduler) syncAudits(ctx context.Context) {
	pending, err := s.container.Env.Repos.Audits.PendingAppids(ctx)
	if err != nil {
		log.Printf("[scheduler] 读取审核中的小程序失败: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	summary, err := s.container.Audit.Sync(ctx, gen.AppidSelection{Appids: &pending})
	if err != nil {
		log.Printf("[scheduler] 审核状态对账失败: %v", err)
		return
	}
	if summary.Total > 0 {
		log.Printf("[scheduler] 审核状态对账完成：共 %d 个，成功 %d，失败 %d", summary.Total, summary.Succeeded, summary.Failed)
	}
}

// syncAuthorizers 定期刷新授权方资料（昵称/头像/权限集/类目等快照）。
func (s *Scheduler) syncAuthorizers(ctx context.Context) {
	summary, err := s.container.Authorizer.SyncMany(ctx, gen.AppidSelection{})
	if err != nil {
		log.Printf("[scheduler] 授权方信息同步失败: %v", err)
		return
	}
	if summary.Failed > 0 {
		log.Printf("[scheduler] 授权方信息同步：共 %d 个，成功 %d，失败 %d", summary.Total, summary.Succeeded, summary.Failed)
	}
}

// checkTicket 检查票据新鲜度；不新鲜时明确告警并给出处置建议。
func (s *Scheduler) checkTicket(ctx context.Context) {
	status := s.container.Env.Platform
	out, err := status.Status(ctx)
	if err != nil {
		return
	}
	if out.ComponentAppid == "" {
		return
	}
	if out.TicketFresh {
		return
	}
	age := int64(-1)
	if out.TicketAgeSeconds != nil {
		age = *out.TicketAgeSeconds
	}
	log.Printf("[scheduler] 告警：component_verify_ticket 不新鲜（距今 %d 秒）。"+
		"请检查开放平台后台「授权事件接收 URL」是否指向 %s、服务器出口 IP 是否已在白名单（未加白返回 61004）、"+
		"以及回调是否被防火墙拦截；也可调用 api_start_push_ticket 恢复推送。当前平台状态：%+v",
		age, out.AuthorizationEventUrl, out.Warnings)
}

// pruneLogs 按保留天数清理日志表。
func (s *Scheduler) pruneLogs(ctx context.Context) {
	settings, err := s.container.Env.Settings.Get()
	if err != nil {
		return
	}
	days := settings.LogRetentionDays
	if days <= 0 {
		return
	}
	before := time.Now().AddDate(0, 0, -days)
	if n, err := s.container.Env.Repos.ApiCalls.PruneBefore(ctx, before); err != nil {
		log.Printf("[scheduler] 清理微信调用日志失败: %v", err)
	} else if n > 0 {
		log.Printf("[scheduler] 已清理 %d 条微信调用日志（保留 %d 天）", n, days)
	}
	if n, err := s.container.Env.Repos.Callbacks.PruneBefore(ctx, before); err != nil {
		log.Printf("[scheduler] 清理回调事件失败: %v", err)
	} else if n > 0 {
		log.Printf("[scheduler] 已清理 %d 条回调事件（保留 %d 天）", n, days)
	}
}

// WriteOperationLog 供各子系统记录平台操作（撤回、发布、授权变更等）。
func (c *Container) WriteOperationLog(ctx context.Context, action, targetType, targetID string, detail map[string]any, actor, ip string) {
	entry := &model.OperationLog{
		Actor:      actor,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		IP:         ip,
	}
	if detail != nil {
		entry.Detail = model.JSONMap(detail)
	}
	if entry.Actor == "" {
		entry.Actor = "admin"
	}
	if err := c.Env.Repos.Operations.Create(ctx, entry); err != nil {
		log.Printf("[operation] 写操作日志失败(action=%s target=%s): %v", action, targetID, err)
	}
}
