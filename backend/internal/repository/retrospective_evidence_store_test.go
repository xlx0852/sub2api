package repository

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetrospectiveEvidenceStoreIntegration(t *testing.T) {
	if os.Getenv("RETROSPECTIVE_RISK_OSS_INTEGRATION") != "1" {
		t.Skip("set RETROSPECTIVE_RISK_OSS_INTEGRATION=1")
	}
	store := NewContentModerationEvidenceStore()
	require.True(t, store.Configured())
	require.NoError(t, store.Probe(context.Background()))
	_, err := store.List(context.Background(), "", 10)
	require.NoError(t, err)
}

func TestRetrospectiveEvidenceEnvelopeRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	objectKey := "risk-evidence/pending/2026/08/20/ab/evidence.json.zst.enc"
	plaintext := bytes.Repeat([]byte("sensitive client content "), 200)

	envelope, err := encryptRetrospectiveEvidence(key, objectKey, "v1", plaintext)
	require.NoError(t, err)
	require.NotContains(t, string(envelope), "sensitive client content")

	got, err := decryptRetrospectiveEvidence(key, objectKey, "v1", envelope)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}

func TestRetrospectiveEvidenceEnvelopeBindsObjectKeyAndVersion(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	envelope, err := encryptRetrospectiveEvidence(key, "risk-evidence/pending/a", "v1", []byte("payload"))
	require.NoError(t, err)

	_, err = decryptRetrospectiveEvidence(key, "risk-evidence/flagged/a", "v1", envelope)
	require.Error(t, err)
	_, err = decryptRetrospectiveEvidence(key, "risk-evidence/pending/a", "v2", envelope)
	require.Error(t, err)

	tampered := append([]byte(nil), envelope...)
	tampered[len(tampered)-1] ^= 0xff
	_, err = decryptRetrospectiveEvidence(key, "risk-evidence/pending/a", "v1", tampered)
	require.Error(t, err)
}

func TestDecodeEvidenceKeyRejectsInvalidLength(t *testing.T) {
	_, err := decodeEvidenceKey("c2hvcnQ=")
	require.Error(t, err)
}

func TestEvidenceStoreLoadsCurrentAndPreviousRotationKeys(t *testing.T) {
	v1 := bytes.Repeat([]byte{1}, 32)
	v2 := bytes.Repeat([]byte{2}, 32)
	t.Setenv("RETROSPECTIVE_RISK_EVIDENCE_KEY_VERSION", "v2")
	t.Setenv("RETROSPECTIVE_RISK_EVIDENCE_KEY_V1", base64.StdEncoding.EncodeToString(v1))
	t.Setenv("RETROSPECTIVE_RISK_EVIDENCE_KEY_V2", base64.StdEncoding.EncodeToString(v2))
	store, ok := NewContentModerationEvidenceStore().(*retrospectiveEvidenceStore)
	require.True(t, ok)
	require.Equal(t, "v2", store.CurrentKeyVersion())
	require.Equal(t, v1, store.keys["v1"])
	require.Equal(t, v2, store.keys["v2"])
}

func TestEvidenceStoreFailsClosedWithoutCredentials(t *testing.T) {
	t.Setenv("RETROSPECTIVE_RISK_OSS_BUCKET", "")
	store := NewContentModerationEvidenceStore()
	require.False(t, store.Configured())
	_, err := store.Put(context.Background(), "risk-evidence/pending/a", "v1", []byte("secret"))
	require.Error(t, err)
	require.Error(t, store.Probe(context.Background()))
}
