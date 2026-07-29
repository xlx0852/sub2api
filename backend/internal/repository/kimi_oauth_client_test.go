package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kimi"
	"github.com/stretchr/testify/require"
)

func TestKimiRequestHeadersUseOfficialCLIIdentityAndStableDevice(t *testing.T) {
	headers := kimiRequestHeaders(kimi.DeviceHeaders{DeviceID: "device-1", DeviceName: "host-1", DeviceModel: "Linux arm64"})
	require.Equal(t, "KimiCLI/1.10.6", headers["User-Agent"])
	require.Equal(t, "kimi_cli", headers["X-Msh-Platform"])
	require.Equal(t, "1.10.6", headers["X-Msh-Version"])
	require.Equal(t, "device-1", headers["X-Msh-Device-Id"])
}
