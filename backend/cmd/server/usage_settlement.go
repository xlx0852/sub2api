package main

import (
	appbilling "github.com/Wei-Shaw/sub2api/internal/app/billing"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageSettlementBindings struct {
	worker *appbilling.DurableSettlement
}

func provideUsageSettlementBindings(
	gateway *service.GatewayService,
	openAI *service.OpenAIGatewayService,
	worker *appbilling.DurableSettlement,
	postCommit *service.UsageSettlementPostCommit,
	recovery *service.UsageSettlementRecovery,
) *usageSettlementBindings {
	gateway.SetUsageSettlementCoordinator(worker)
	openAI.SetUsageSettlementCoordinator(worker)
	worker.SetAppliedObserver(postCommit)
	worker.SetTerminalRecoverer(recovery)
	return &usageSettlementBindings{worker: worker}
}
