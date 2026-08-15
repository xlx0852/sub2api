package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelPricingRestrictionThinEntrypointsStayEquivalentOnNoopInputs(t *testing.T) {
	ctx := context.Background()
	gateway := &GatewayService{}
	openAI := &OpenAIGatewayService{}
	groupID := int64(7)

	for _, input := range []struct {
		groupID *int64
		model   string
	}{
		{groupID: nil, model: "model"},
		{groupID: &groupID, model: ""},
		{groupID: &groupID, model: "model"},
	} {
		require.Equal(t,
			gateway.checkChannelPricingRestriction(ctx, input.groupID, input.model),
			openAI.checkChannelPricingRestriction(ctx, input.groupID, input.model),
		)
	}
}
