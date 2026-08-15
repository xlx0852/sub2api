package billing

import "github.com/google/wire"

var ProviderSet = wire.NewSet(ProvideDurableSettlement)
