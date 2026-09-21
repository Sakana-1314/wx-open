package service

import (
	"context"
	"log"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
)

// LogService 提供三类日志的查询：微信调用日志、回调事件、平台操作日志。
type LogService struct {
	repos *core.Repos
}

// ApiCalls 微信接口调用日志（排障入口：请求/响应与返回码分类都在这里）。
func (s *LogService) ApiCalls(ctx context.Context, params gen.ListApiCallLogsParams) (*gen.ApiCallLogListResponse, error) {
	filter := repo.ApiCallLogFilter{
		Page:     pageOf(params.Page),
		PageSize: pageSizeOf(params.PageSize),
	}
	if params.Appid != nil {
		filter.Appid = *params.Appid
	}
	if params.JobId != nil {
		filter.JobID = *params.JobId
	}
	if params.Endpoint != nil {
		filter.Endpoint = *params.Endpoint
	}
	if params.Errcode != nil {
		code := *params.Errcode
		filter.Errcode = &code
	}
	items, total, err := s.repos.ApiCalls.List(ctx, filter)
	if err != nil {
		return nil, core.Internal(err)
	}
	out := &gen.ApiCallLogListResponse{
		Items:    make([]gen.ApiCallLog, 0, len(items)),
		Total:    int(total),
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}
	for i := range items {
		out.Items = append(out.Items, toAPICallLog(&items[i]))
	}
	return out, nil
}

// Callbacks 回调事件（含解密后的原文，便于核对微信推送内容）。
func (s *LogService) Callbacks(ctx context.Context, params gen.ListCallbackEventsParams) (*gen.CallbackEventListResponse, error) {
	page, pageSize := pageOf(params.Page), pageSizeOf(params.PageSize)
	kind := model.CallbackKind("")
	if params.Kind != nil {
		kind = model.CallbackKind(*params.Kind)
	}
	appid, infoType := "", ""
	if params.Appid != nil {
		appid = *params.Appid
	}
	if params.InfoType != nil {
		infoType = *params.InfoType
	}
	items, total, err := s.repos.Callbacks.List(ctx, kind, appid, infoType, page, pageSize)
	if err != nil {
		return nil, core.Internal(err)
	}
	out := &gen.CallbackEventListResponse{
		Items:    make([]gen.CallbackEvent, 0, len(items)),
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range items {
		ev := items[i]
		row := gen.CallbackEvent{
			Id:          int64(ev.ID),
			Kind:        gen.CallbackKind(ev.Kind),
			SignatureOk: ev.SignatureOK,
			Processed:   ev.Processed,
			ReceivedAt:  ev.ReceivedAt,
		}
		if ev.Appid != "" {
			row.Appid = &ev.Appid
		}
		if ev.InfoType != "" {
			row.InfoType = &ev.InfoType
		}
		if ev.Event != "" {
			row.Event = &ev.Event
		}
		if ev.Decrypted != "" {
			row.Decrypted = &ev.Decrypted
		}
		if ev.ProcessNote != "" {
			row.ProcessNote = &ev.ProcessNote
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

// Operations 平台操作日志。
func (s *LogService) Operations(ctx context.Context, params gen.ListOperationLogsParams) (*gen.OperationLogListResponse, error) {
	page, pageSize := pageOf(params.Page), pageSizeOf(params.PageSize)
	action := ""
	if params.Action != nil {
		action = *params.Action
	}
	items, total, err := s.repos.Operations.List(ctx, action, page, pageSize)
	if err != nil {
		return nil, core.Internal(err)
	}
	out := &gen.OperationLogListResponse{
		Items:    make([]gen.OperationLog, 0, len(items)),
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range items {
		row := items[i]
		entry := gen.OperationLog{
			Id:        int64(row.ID),
			Actor:     row.Actor,
			Action:    row.Action,
			CreatedAt: row.CreatedAt,
		}
		if row.TargetType != "" {
			entry.TargetType = &row.TargetType
		}
		if row.TargetID != "" {
			entry.TargetId = &row.TargetID
		}
		if row.IP != "" {
			entry.Ip = &row.IP
		}
		if row.Detail != nil {
			detail := map[string]any(row.Detail)
			entry.Detail = &detail
		}
		out.Items = append(out.Items, entry)
	}
	return out, nil
}

// WxCallRecorder 把每次微信调用写入 api_call_logs（wxapi.CallLogger 的实现）。
type WxCallRecorder struct {
	Repos *core.Repos
}

// LogCall 实现 wxapi.CallLogger。
func (r *WxCallRecorder) LogCall(rec wxapi.CallRecord) {
	if r == nil || r.Repos == nil {
		return
	}
	entry := &model.ApiCallLog{
		Appid:      rec.Appid,
		Scope:      rec.Scope,
		Endpoint:   rec.Endpoint,
		Method:     rec.Method,
		HTTPStatus: rec.HTTPStatus,
		OK:         rec.OK,
		Errcode:    rec.Errcode,
		Errmsg:     truncateString(rec.Errmsg, 900),
		ErrorClass: model.ClassifyErrcode(rec.Errcode),
		DurationMs: rec.DurationMs,
		Request:    model.JSONMap(rec.Request),
		Response:   model.JSONMap(rec.Response),
	}
	if rec.OK {
		entry.ErrorClass = ""
	}
	if jobID, ok := rec.Request["__job_id"].(string); ok {
		entry.JobID = jobID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.Repos.ApiCalls.Create(ctx, entry); err != nil {
		log.Printf("[wxapi] 写入调用日志失败: %v", err)
	}
}

func toAPICallLog(row *model.ApiCallLog) gen.ApiCallLog {
	duration := row.DurationMs
	out := gen.ApiCallLog{
		Id:         int64(row.ID),
		Scope:      gen.TokenScope(row.Scope),
		Endpoint:   row.Endpoint,
		Method:     &row.Method,
		HttpStatus: &row.HTTPStatus,
		Ok:         row.OK,
		Errcode:    &row.Errcode,
		DurationMs: &duration,
		CreatedAt:  row.CreatedAt,
	}
	if row.JobID != "" {
		out.JobId = &row.JobID
	}
	if row.Appid != "" {
		out.Appid = &row.Appid
	}
	if row.Errmsg != "" {
		out.Errmsg = &row.Errmsg
	}
	if row.ErrorClass != "" {
		class := gen.ErrorClass(row.ErrorClass)
		out.ErrorClass = &class
	}
	if row.Request != nil {
		req := map[string]any(row.Request)
		out.Request = &req
	}
	if row.Response != nil {
		resp := map[string]any(row.Response)
		out.Response = &resp
	}
	return out
}

func pageOf(page *int) int {
	if page == nil || *page < 1 {
		return 1
	}
	return *page
}

func pageSizeOf(size *int) int {
	if size == nil || *size < 1 {
		return 20
	}
	if *size > 200 {
		return 200
	}
	return *size
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
