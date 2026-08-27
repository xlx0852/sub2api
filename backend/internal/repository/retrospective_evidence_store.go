package repository

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
)

const (
	retrospectiveEvidenceMagic     = "S2RE1"
	retrospectiveEvidenceMaxObject = 2 << 20
)

type retrospectiveEvidenceStore struct {
	client     *oss.Client
	bucket     string
	prefix     string
	keys       map[string][]byte
	keyVersion string
	configured bool
}

func NewContentModerationEvidenceStore() ports.ContentModerationEvidenceStore {
	bucket := strings.TrimSpace(os.Getenv("RETROSPECTIVE_RISK_OSS_BUCKET"))
	region := strings.TrimSpace(os.Getenv("RETROSPECTIVE_RISK_OSS_REGION"))
	endpoint := strings.TrimSpace(os.Getenv("RETROSPECTIVE_RISK_OSS_ENDPOINT"))
	prefix := normalizeEvidencePrefix(os.Getenv("RETROSPECTIVE_RISK_OSS_PREFIX"))
	accessKeyID := strings.TrimSpace(os.Getenv("RETROSPECTIVE_RISK_OSS_ACCESS_KEY_ID"))
	accessKeySecret := strings.TrimSpace(os.Getenv("RETROSPECTIVE_RISK_OSS_ACCESS_KEY_SECRET"))
	keyVersion := strings.TrimSpace(os.Getenv("RETROSPECTIVE_RISK_EVIDENCE_KEY_VERSION"))
	if keyVersion == "" {
		keyVersion = "v1"
	}
	keyMaterial, _ := decodeEvidenceKey(os.Getenv("RETROSPECTIVE_RISK_EVIDENCE_KEY_" + strings.ToUpper(keyVersion)))
	store := &retrospectiveEvidenceStore{
		bucket:     bucket,
		prefix:     prefix,
		keys:       map[string][]byte{},
		keyVersion: keyVersion,
	}
	for _, version := range []string{"v1", "v2", "v3", keyVersion} {
		if _, exists := store.keys[version]; exists {
			continue
		}
		material, err := decodeEvidenceKey(os.Getenv("RETROSPECTIVE_RISK_EVIDENCE_KEY_" + strings.ToUpper(version)))
		if err == nil {
			store.keys[version] = material
		}
	}
	if bucket == "" || region == "" || endpoint == "" || accessKeyID == "" || accessKeySecret == "" || len(keyMaterial) != 32 {
		return store
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")
	provider := credentials.NewStaticCredentialsProvider(accessKeyID, accessKeySecret)
	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(provider).
		WithRegion(strings.TrimPrefix(region, "oss-")).
		WithEndpoint(endpoint).
		WithConnectTimeout(5 * time.Second).
		WithReadWriteTimeout(15 * time.Second).
		WithRetryMaxAttempts(3)
	store.client = oss.NewClient(cfg)
	store.configured = true
	return store
}

func normalizeEvidencePrefix(raw string) string {
	prefix := strings.Trim(strings.TrimSpace(raw), "/")
	if prefix == "" {
		prefix = "risk-evidence"
	}
	return prefix + "/"
}

func decodeEvidenceKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty evidence key")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode evidence key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("evidence key must contain 32 bytes")
	}
	return key, nil
}

func (s *retrospectiveEvidenceStore) Configured() bool {
	return s != nil && s.configured && s.client != nil
}

func (s *retrospectiveEvidenceStore) CurrentKeyVersion() string {
	if s == nil {
		return ""
	}
	return s.keyVersion
}

func (s *retrospectiveEvidenceStore) validateObjectKey(objectKey string) error {
	if !s.Configured() {
		return errors.New("retrospective evidence store is not configured")
	}
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" || !strings.HasPrefix(objectKey, s.prefix) || strings.Contains(objectKey, "..") || strings.ContainsAny(objectKey, "\r\n") {
		return errors.New("retrospective evidence object key is outside configured prefix")
	}
	return nil
}

func (s *retrospectiveEvidenceStore) Put(ctx context.Context, objectKey, keyVersion string, plaintext []byte) (int64, error) {
	if err := s.validateObjectKey(objectKey); err != nil {
		return 0, err
	}
	key, ok := s.keys[strings.TrimSpace(keyVersion)]
	if !ok {
		return 0, errors.New("retrospective evidence encryption key version is unavailable")
	}
	envelope, err := encryptRetrospectiveEvidence(key, objectKey, keyVersion, plaintext)
	if err != nil {
		return 0, err
	}
	digest := md5.Sum(envelope)
	contentMD5 := base64.StdEncoding.EncodeToString(digest[:])
	contentLength := int64(len(envelope))
	contentType := "application/octet-stream"
	forbidOverwrite := "true"
	serverSideEncryption := "AES256"
	_, err = s.client.PutObject(ctx, &oss.PutObjectRequest{
		Bucket:               oss.Ptr(s.bucket),
		Key:                  oss.Ptr(objectKey),
		Body:                 bytes.NewReader(envelope),
		ContentLength:        &contentLength,
		ContentMD5:           &contentMD5,
		ContentType:          &contentType,
		ForbidOverwrite:      &forbidOverwrite,
		ServerSideEncryption: &serverSideEncryption,
		Metadata: map[string]string{
			"key-version": keyVersion,
			"format":      "s2re1",
		},
	})
	if err != nil {
		return 0, fmt.Errorf("put retrospective evidence object: %w", err)
	}
	return contentLength, nil
}

func (s *retrospectiveEvidenceStore) Get(ctx context.Context, objectKey, keyVersion string) ([]byte, error) {
	if err := s.validateObjectKey(objectKey); err != nil {
		return nil, err
	}
	key, ok := s.keys[strings.TrimSpace(keyVersion)]
	if !ok {
		return nil, errors.New("retrospective evidence encryption key version is unavailable")
	}
	result, err := s.client.GetObject(ctx, &oss.GetObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(objectKey)})
	if err != nil {
		return nil, fmt.Errorf("get retrospective evidence object: %w", err)
	}
	defer func() { _ = result.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(result.Body, retrospectiveEvidenceMaxObject+1))
	if err != nil {
		return nil, fmt.Errorf("read retrospective evidence object: %w", err)
	}
	if len(raw) > retrospectiveEvidenceMaxObject {
		return nil, errors.New("retrospective evidence object exceeds size limit")
	}
	plaintext, err := decryptRetrospectiveEvidence(key, objectKey, keyVersion, raw)
	if err != nil {
		return nil, fmt.Errorf("decrypt retrospective evidence object: %w", err)
	}
	return plaintext, nil
}

func (s *retrospectiveEvidenceStore) Delete(ctx context.Context, objectKey string) error {
	if err := s.validateObjectKey(objectKey); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &oss.DeleteObjectRequest{Bucket: oss.Ptr(s.bucket), Key: oss.Ptr(objectKey)})
	if err != nil {
		return fmt.Errorf("delete retrospective evidence object: %w", err)
	}
	return nil
}

func (s *retrospectiveEvidenceStore) List(ctx context.Context, cursor string, limit int) (domain.EvidenceObjectPage, error) {
	if !s.Configured() {
		return domain.EvidenceObjectPage{}, errors.New("retrospective evidence store is not configured")
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	request := &oss.ListObjectsV2Request{Bucket: oss.Ptr(s.bucket), Prefix: oss.Ptr(s.prefix), MaxKeys: int32(limit)}
	if strings.TrimSpace(cursor) != "" {
		request.ContinuationToken = oss.Ptr(cursor)
	}
	result, err := s.client.ListObjectsV2(ctx, request)
	if err != nil {
		return domain.EvidenceObjectPage{}, fmt.Errorf("list retrospective evidence objects: %w", err)
	}
	page := domain.EvidenceObjectPage{Keys: make([]string, 0, len(result.Contents))}
	for _, object := range result.Contents {
		if object.Key != nil {
			page.Keys = append(page.Keys, *object.Key)
		}
	}
	if result.IsTruncated && result.NextContinuationToken != nil {
		page.NextCursor = *result.NextContinuationToken
	}
	return page, nil
}

func (s *retrospectiveEvidenceStore) Probe(ctx context.Context) error {
	if !s.Configured() {
		return errors.New("retrospective evidence store is not configured")
	}
	keyVersion := s.CurrentKeyVersion()
	objectKey := s.prefix + "probe/runtime-" + uuid.NewString() + ".bin"
	written, err := s.Put(ctx, objectKey, keyVersion, []byte("retrospective-risk-probe"))
	if err != nil {
		return err
	}
	if written <= 0 {
		return errors.New("retrospective evidence probe wrote zero bytes")
	}
	deleted := false
	defer func() {
		if !deleted {
			_ = s.Delete(context.Background(), objectKey)
		}
	}()
	plaintext, err := s.Get(ctx, objectKey, keyVersion)
	if err != nil {
		return err
	}
	if string(plaintext) != "retrospective-risk-probe" {
		return errors.New("retrospective evidence probe content mismatch")
	}
	if err := s.Delete(ctx, objectKey); err != nil {
		return err
	}
	deleted = true
	return nil
}

func encryptRetrospectiveEvidence(key []byte, objectKey, keyVersion string, plaintext []byte) ([]byte, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("create evidence compressor: %w", err)
	}
	compressed := encoder.EncodeAll(plaintext, nil)
	encoder.Close()
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create evidence cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create evidence gcm: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("create evidence nonce: %w", err)
	}
	aad := []byte(objectKey + "\n" + keyVersion)
	ciphertext := aead.Seal(nil, nonce, compressed, aad)
	out := make([]byte, 0, len(retrospectiveEvidenceMagic)+len(nonce)+len(ciphertext))
	out = append(out, retrospectiveEvidenceMagic...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

func decryptRetrospectiveEvidence(key []byte, objectKey, keyVersion string, envelope []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	headerLen := len(retrospectiveEvidenceMagic) + aead.NonceSize()
	if len(envelope) <= headerLen || string(envelope[:len(retrospectiveEvidenceMagic)]) != retrospectiveEvidenceMagic {
		return nil, errors.New("invalid retrospective evidence envelope")
	}
	nonce := envelope[len(retrospectiveEvidenceMagic):headerLen]
	ciphertext := envelope[headerLen:]
	compressed, err := aead.Open(nil, nonce, ciphertext, []byte(objectKey+"\n"+keyVersion))
	if err != nil {
		return nil, err
	}
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	plaintext, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		return nil, err
	}
	if len(plaintext) > 128<<10 {
		return nil, errors.New("retrospective evidence plaintext exceeds size limit")
	}
	return plaintext, nil
}
