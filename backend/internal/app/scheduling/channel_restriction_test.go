package scheduling

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillingModelForRestriction(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{name: "requested", source: BillingModelSourceRequested, want: "requested-model"},
		{name: "channel mapped", source: BillingModelSourceChannelMapped, want: "mapped-model"},
		{name: "upstream deferred", source: BillingModelSourceUpstream, want: ""},
		{name: "legacy empty defaults mapped", source: "", want: "mapped-model"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, BillingModelForRestriction(test.source, "requested-model", "mapped-model"))
		})
	}
}
