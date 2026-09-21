// Package wxjob 实现批量作业子系统：作业的创建 / 预览 / 查询 / 控制，以及
// 「上传代码(commit) → 隐私检测(privacy_check) → 提审(submit_audit) → 发布(release)」
// 四类步骤的执行器，并给 batch.Engine 提供持久化适配（EngineStore）。
//
// # 与引擎的分工
//
// 引擎（internal/batch）负责调度：步骤串行、同一 appid 串行、分类重试、waiting 轮询、
// 断点续跑、单实例锁。本包只负责「一件事该怎么调微信、结果怎么解释、参数怎么校验」：
//
//   - 执行器（CommitExecutor / PrivacyCheckExecutor / SubmitAuditExecutor / ReleaseExecutor /
//     SingleStepExecutor）实现 batch.Executor，构造函数签名见下；
//   - EngineStore 把 repo 的 Jobs/JobItems 适配成 batch.Store（含 repo.ErrNotFound → batch.ErrNoItem）；
//   - JobService 承载 Preview/Create/List/Get/Items/Start/Pause/Resume/Cancel/RetryFailed。
//
// 官方硬性约束在本包中的落点：
//
//   - 上传代码成功后**绝不**在同一步里提审：隐私检测是独立步骤（step 门控由引擎保证），
//     否则会反复触发 61039；
//   - 同一小程序提交有并发限制（9402202）：创建作业时对同类步骤做在途冲突检查，冲突项标 skipped；
//   - 提审额度是服务商级共享（85085）：提审前读额度缓存，为 0 或收到 85085 时返回 Fatal=true 暂停整个作业；
//   - 提审前必须有「成功上传代码」的记录（平台侧预检，避免用户以为是微信返回的 85086）：
//     ① 本作业内存在成功的 commit 子项；或 ② 该 appid 最近 30 天内在本平台有成功的 commit 记录
//     （crossJobCommitWindow，覆盖「先跑上传作业、再单独跑提审作业」的真实用法）；
//     或 ③ authorizer.CodeSource == direct_commit（代码由 CI 直传）；三条都不满足才失败；
//   - 标准模板官方已下架（9402203）：只允许 template_type==0 的普通模板；
//   - user_version ≤ 64 字符、ext_json 必须是字符串化 JSON 且不能残留占位符。
//
// # 单步作业的取参规则
//
//   - sync_audit_status：查 get_latest_auditstatus 并 upsert 到 audit_records（Source=poll）；没有审核单则跳过；
//   - undo_audit：撤回当前审核中的版本，不需要 auditId；先用本地用量台账拦每天 5 次 / 每月 10 次（87013）；
//   - speedup_audit：用该小程序**最近一次审核单**的 auditId（env.Repos.Audits.LatestByApp），
//     没有审核单则失败并说明；收到 89405（加急额度用尽）返回 Fatal 暂停作业；
//   - revert：不传 app_version 即回退到上一个版本（官方默认行为）；87012 按永久失败处理并说明；
//   - toggle_visit：只读作业载荷的 pause_service（来自契约字段 gen.JobCreateRequest.PauseService，
//     true=暂停服务/close，false=恢复服务/open）；缺失则失败并提示「未指定服务状态（pauseService）」，
//     绝不猜默认值（以免误停线上小程序）；
//   - sync_info / set_domain 等其余类型返回 skipped（由各自的专用页面执行）。
//
// # 作业载荷（batch_jobs.payload）
//
// 载荷保存「足够复现请求体」的全部信息，执行器只读载荷、不依赖内存状态：
//
//	{
//	  "type": "pipeline",
//	  "appids": ["wx..."],
//	  "selection": {"appids": [...]},
//	  "commit":  {"template_id": 12, "ext_template": "", "ext_overrides": {}, "user_version_pattern": "{{date}}", "user_desc_pattern": ""},
//	  "audit":   {"profile_id": 1, "version_desc_pattern": "", "privacy_api_not_use": null, "order_path": ""},
//	  "release": {"gray_percentage": null, "support_debuger_first": false, "support_experiencer_first": false},
//	  "single":  {"audit_id": 0, "app_version": null, "visit_action": ""},
//	  "pause_service": null
//	}
//
// # DryRun 语义
//
// DryRun=true 时只落库不执行：作业直接置为 succeeded（引擎只扫描 pending/running，不会捡起它），
// 校验通过的子项置为 succeeded 并在 request 里留下计划请求体，校验不通过的子项置为 skipped 并写明原因。
//
// # 装配（service.Container）
//
//   - NewJobService(env) 只依赖 core.Env；微信调用日志由 wxapi.Client 的 Logger（service.WxCallRecorder）统一落库；
//   - AttachEngine(engine) 必须在容器注册完五个执行器后调用一次，否则作业的启动/暂停/恢复/取消/重试
//     都会返回 ErrEngineNotRunning（引擎没拿到单实例锁时同理，此时作业只能创建与查看，不能推进）。
//
// # 未注入引擎时的行为
//
// Start/Pause/Resume/Cancel/RetryFailed 依赖 batch.Engine（由 service.Container 通过
// AttachEngine 注入）。未注入时统一返回 ErrEngineNotRunning（同时命中 core.ErrConflict），
// 提示「作业引擎未启动（可能未取得单实例锁）」。
package wxjob
