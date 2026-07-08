package xai

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func testValidGrokEncryptedContent() string {
	buf := make([]byte, 0, 256)
	for i := 0; len(buf) < 256; i++ {
		sum := sha256.Sum256([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		buf = append(buf, sum[:]...)
	}
	return base64.RawStdEncoding.EncodeToString(buf[:256])
}

func TestInspectGrokEncryptedContent_AcceptsNativeShape(t *testing.T) {
	sample := testValidGrokEncryptedContent()
	if _, err := InspectGrokEncryptedContent(sample); err != nil {
		t.Fatalf("InspectGrokEncryptedContent() error = %v", err)
	}
}

func TestInspectGrokEncryptedContent_RejectsCodexSignature(t *testing.T) {
	_, err := InspectGrokEncryptedContent("gAAAAABinvalid-codex-shape")
	if err == nil {
		t.Fatal("expected Codex gAAAA prefix to be rejected")
	}
}

func TestInspectGrokEncryptedContent_RejectsLowEntropyPayload(t *testing.T) {
	sample := base64.RawStdEncoding.EncodeToString([]byte("short-low-entropy-payload-not-valid"))
	_, err := InspectGrokEncryptedContent(sample)
	if err == nil {
		t.Fatal("expected low-entropy payload to be rejected")
	}
}