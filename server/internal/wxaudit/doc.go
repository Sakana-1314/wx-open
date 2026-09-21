// Package wxaudit 实现「审核 / 发布 / 前置体检」三个业务子系统：
//
//   - AuditService：审核单台账（列表、历史）、审核状态对账（Sync）、
//     审核结果事件推送（OnAuditResult，实现 callback.BusinessHandler 的对应方法）、
//     撤回（Undo）、加急（SpeedUp）、服务商级额度（Quota）；
//   - ReleaseService：发布台账、体验版/线上版信息、全量发布、分阶段发布（灰度）、
//     灰度计划、取消灰度、可回退版本列表、版本回退、体验版二维码；
//   - PreflightService：批量下发/提审/发布前的逐项体检（授权、权限集、资料完整性、
//     类目、隐私指引、域名、额度、模板库）。
//
// 分层约定（与 AGENTS.md 一致）：
//   - 本包只依赖 core / repo / model / wxapi / wxtoken / gen，不 import handler、service；
//   - 所有微信调用一律经 wxapi（令牌经 wxtoken 获取），不在本包直接发 HTTP；
//   - 不写 SQL：数据访问一律走 env.Repos 的强类型方法；
//   - 业务错误用 core 的哨兵错误包装；微信侧失败统一转 core.WeChatError（保留 errcode，
//     并按接口覆盖 model 的通用处置建议，给出可执行的中文动作建议）。
//
// 与官方文档对应的硬性规则（实现处均有注释标注依据）：
//
//	提审/加急额度是**服务商级、旗下小程序共用**（QueryQuota 的返回与保存见 Quota 与 Preflight）；
//	撤回审核每个账号每天 ≤5 次、每月 ≤10 次，超限微信返回 87013（先在本地拦截）；
//	发布的是「最后一个审核通过的版本」，且为全量发布、立即生效（85019/85020/85021）；
//	灰度比例必须是 0-100 整数，为 0 时必须指定项目成员或体验成员优先（85079~85082）；
//	版本回退最多保留最近 5 个发布/回退版本，无上一个线上版本时无法回退（87012）。
package wxaudit
