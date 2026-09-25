package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

// MergeFirstOutputStages 把请求耗时采集器产出的新分段合并进旧 6 段快照。
//
// 约定：旧字段以 base 为准（保持历史语义），新字段只在 base 为空时填充；
// 两侧都为 nil 时返回 nil（前端所有行显示「未采集」）。
func MergeFirstOutputStages(base, detail *domain.FirstOutputStages) *domain.FirstOutputStages {
	if base == nil {
		return detail
	}
	if detail == nil {
		return base
	}
	if base.BodyFirstWaitMs == nil {
		base.BodyFirstWaitMs = detail.BodyFirstWaitMs
	}
	if base.BodyReceiveMs == nil {
		base.BodyReceiveMs = detail.BodyReceiveMs
	}
	if base.BodyReadMs == nil {
		base.BodyReadMs = detail.BodyReadMs
	}
	if base.BodyBytes == nil {
		base.BodyBytes = detail.BodyBytes
	}
	if base.BodyComplete == nil {
		base.BodyComplete = detail.BodyComplete
	}
	if base.UpstreamRequestBytes == nil {
		base.UpstreamRequestBytes = detail.UpstreamRequestBytes
	}
	if base.APIKeyAuthMs == nil {
		base.APIKeyAuthMs = detail.APIKeyAuthMs
	}
	if base.SecurityAuditMs == nil {
		base.SecurityAuditMs = detail.SecurityAuditMs
	}
	if base.UserQueueMs == nil {
		base.UserQueueMs = detail.UserQueueMs
	}
	if base.AccountSelectMs == nil {
		base.AccountSelectMs = detail.AccountSelectMs
	}
	if base.AccountQueueMs == nil {
		base.AccountQueueMs = detail.AccountQueueMs
	}
	if base.HandlerBodyReadMs == nil {
		base.HandlerBodyReadMs = detail.HandlerBodyReadMs
	}
	if base.HandlerTotalMs == nil {
		base.HandlerTotalMs = detail.HandlerTotalMs
	}
	if base.UpstreamAttempts == nil {
		base.UpstreamAttempts = detail.UpstreamAttempts
	}
	if base.ConnectionReused == nil {
		base.ConnectionReused = detail.ConnectionReused
	}
	if base.UpstreamWriteMs == nil {
		base.UpstreamWriteMs = detail.UpstreamWriteMs
	}
	if base.UpstreamWroteHeadersMs == nil {
		base.UpstreamWroteHeadersMs = detail.UpstreamWroteHeadersMs
	}
	if base.UpstreamDialMs == nil {
		base.UpstreamDialMs = detail.UpstreamDialMs
	}
	if base.UpstreamTLSMs == nil {
		base.UpstreamTLSMs = detail.UpstreamTLSMs
	}
	if base.UpstreamDialed == nil {
		base.UpstreamDialed = detail.UpstreamDialed
	}
	if base.UpstreamProto == "" {
		base.UpstreamProto = detail.UpstreamProto
	}
	if base.UpstreamDoMs == nil {
		base.UpstreamDoMs = detail.UpstreamDoMs
	}
	if base.UpstreamBuildMs == nil {
		base.UpstreamBuildMs = detail.UpstreamBuildMs
	}
	if base.UpstreamFirstByteMs == nil {
		base.UpstreamFirstByteMs = detail.UpstreamFirstByteMs
	}
	if base.UpstreamResponseHeadersMs == nil {
		base.UpstreamResponseHeadersMs = detail.UpstreamResponseHeadersMs
	}
	if base.UpstreamStreamTransferMs == nil {
		base.UpstreamStreamTransferMs = detail.UpstreamStreamTransferMs
	}
	if base.ResponseStreamComplete == nil {
		base.ResponseStreamComplete = detail.ResponseStreamComplete
	}
	if base.Outcome == "" {
		base.Outcome = detail.Outcome
	}
	if len(base.Segments) == 0 && len(detail.Segments) > 0 {
		base.Segments = detail.Segments
	}
	return base
}

// mergeRequestTimingStages 把请求级采集器（中间件 + httptrace）的分段合并进网关侧快照。
// 采集器缺失时原样返回 base，旧路径行为不变。
func mergeRequestTimingStages(ctx context.Context, base *domain.FirstOutputStages) *domain.FirstOutputStages {
	collector := requesttiming.FromContext(ctx)
	if collector == nil {
		return base
	}
	return MergeFirstOutputStages(base, collector.Snapshot(time.Now()))
}
