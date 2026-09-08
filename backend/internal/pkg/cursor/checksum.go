package cursor

import (
	"encoding/base64"
	"time"
)

const (
	DefaultBackendURL          = "https://api2.cursor.sh"
	DefaultClientType          = "sand"
	DefaultClientVersion       = "0.27.0"
	DefaultBoxNamespace        = "prod"
	HeaderCursorChecksum       = "x-cursor-checksum"
	HeaderCursorClientType     = "x-cursor-client-type"
	HeaderCursorClientVersion  = "x-cursor-client-version"
	HeaderSandBoxNamespace     = "x-sand-box-namespace"
	HeaderGhostMode            = "x-ghost-mode"
	HeaderRequestID            = "x-request-id"
)

// EnhancedObfuscate performs Cursor's byte-level permutation and XOR obfuscation.
func EnhancedObfuscate(data []byte) []byte {
	out := make([]byte, len(data))
	copy(out, data)
	lastByte := byte(165)
	for i := 0; i < len(out); i++ {
		current := out[i]
		out[i] = ((current ^ lastByte) + byte(i%256)) & 0xFF
		lastByte = out[i]
	}
	return out
}

// CreateCursorChecksum generates the x-cursor-checksum header value for the given machine ID and timestamp.
// Note: In JavaScript (where the algorithm originates), bitwise shifts operate on 32-bit integers
// with the shift count masked by 0x1F (e.g., >> 40 becomes >> 8, and >> 32 becomes >> 0).
func CreateCursorChecksum(machineID string, now time.Time) string {
	unixKiloSeconds := now.UnixMilli() / 1_000_000
	i32 := int32(unixKiloSeconds)
	raw := []byte{
		byte((i32 >> (40 % 32)) & 0xFF),
		byte((i32 >> (32 % 32)) & 0xFF),
		byte((i32 >> 24) & 0xFF),
		byte((i32 >> 16) & 0xFF),
		byte((i32 >> 8) & 0xFF),
		byte(i32 & 0xFF),
	}
	obfuscated := EnhancedObfuscate(raw)
	return base64.RawURLEncoding.EncodeToString(obfuscated) + machineID
}
