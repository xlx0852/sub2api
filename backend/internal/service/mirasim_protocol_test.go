//go:build unit

package service

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

func mirasimTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func mirasimPinnedDeviceKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestMirasimMRS1RoundTrip(t *testing.T) {
	key := mirasimTestKey(t)
	blob, err := mirasimEncryptMRS1("refresh-token-明文", key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(blob, "mrs1:") {
		t.Fatalf("缺少 mrs1: 前缀: %q", blob)
	}
	got, err := mirasimDecryptMRS1(blob, key)
	if err != nil {
		t.Fatal(err)
	}
	if got != "refresh-token-明文" {
		t.Fatalf("往返不一致: %q", got)
	}
	empty, err := mirasimEncryptMRS1("", key)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := mirasimDecryptMRS1(empty, key); err != nil || got != "" {
		t.Fatalf("空串往返失败: %q err=%v", got, err)
	}
}

func TestMirasimMRS1WrongKey(t *testing.T) {
	blob, err := mirasimEncryptMRS1("secret", mirasimTestKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mirasimDecryptMRS1(blob, mirasimTestKey(t)); err == nil {
		t.Fatal("错误 key 应解密失败")
	}
}

func TestMirasimMRS1PlaintextPassthrough(t *testing.T) {
	got, err := mirasimDecryptMRS1("not-encrypted", mirasimTestKey(t))
	if err != nil || got != "not-encrypted" {
		t.Fatalf("非 mrs1: 输入应原样返回, got %q err %v", got, err)
	}
}

// 设备身份黄金向量（与参考实现 internal/mirasim/crypto_test.go 一致）。
func TestMirasimDeviceIdentityPinned(t *testing.T) {
	priv := mirasimPinnedDeviceKey()
	id, pub := mirasimDeviceIdentity(priv)
	if pub != "MCowBQYDK2VwAyEAebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ=" {
		t.Fatalf("publicKeyB64 不符: %q", pub)
	}
	if id != "JUdfAMozPES5oPh9G8P13N" {
		t.Fatalf("deviceID 不符: %q", id)
	}
	if len(id) != 22 {
		t.Fatalf("deviceID 长度应为 22: %d", len(id))
	}
}

func TestMirasimPrivateKeyPEMRoundTrip(t *testing.T) {
	priv, err := mirasimGenerateDeviceKey()
	if err != nil {
		t.Fatal(err)
	}
	pemStr, err := mirasimMarshalPrivateKeyPEM(priv)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mirasimParsePrivateKeyPEM(pemStr)
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed) != string(priv) {
		t.Fatal("PEM 往返后私钥不一致")
	}
	if _, err := mirasimParsePrivateKeyPEM("not a pem"); err == nil {
		t.Fatal("非法 PEM 应报错")
	}
}

// canonical string 精确串断言，期望值来自参考实现（sha256 期望值独立核对过）。
func TestMirasimCanonicalStringExact(t *testing.T) {
	got := mirasimCanonicalString("post", "/v1/messages", 1758000000123, "nonce-abc", "dev-1", "0.0.322",
		"test-credential", nil, []byte("hello body"))
	want := strings.Join([]string{
		"mrs-sig-v2",
		"POST",
		"/v1/messages",
		"1758000000123",
		"nonce-abc",
		"dev-1",
		"0.0.322",
		"a6fbaf094063b9172c9019fa2d20143f8396b431ebe6264ede59bf7c18145a9a",
		"", // 无 meta：第 9 行为空行
		"6d9876f6d571676eb86f735ba9476da91ec5d0c52a69f6434c93f5c9e680210e",
	}, "\n")
	if got != want {
		t.Fatalf("canonical string 不符:\n%q\nwant:\n%q", got, want)
	}
	if n := len(strings.Split(got, "\n")); n != 10 {
		t.Fatalf("应为 10 行，实际 %d", n)
	}
}

func TestMirasimCanonicalStringMetaAndEmptyBody(t *testing.T) {
	got := mirasimCanonicalString("GET", "/v1/limits", 1, "n", "d", "0.0.322",
		"cred", map[string]string{"b": "2", "a": "1"}, nil)
	want := strings.Join([]string{
		"mrs-sig-v2", "GET", "/v1/limits", "1", "n", "d", "0.0.322",
		"55d91a3561684b32df5e58a0d91968b93798af4f924bba383e1c98625ec0c834", // sha256hex("cred")
		"2bc11e86e8348c7c62f11e681435b5d013a262e5a9f3b8ce61c25831a516a45e", // sha256hex("a:1\nb:2") —— 官方 crypto core 语义
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // sha256hex("")
	}, "\n")
	if got != want {
		t.Fatalf("canonical string 不符:\n%q\nwant:\n%q", got, want)
	}
}

// 官方客户端 0.0.260 embedded crypto core 的钉住向量（来源：cpa-plugin-mirasim
// internal/mirasim/protocol_test.go，MIT）。canonical 串与签名必须逐字节一致，
// 否则带元数据的签名会被上游判无效。
func TestMirasimCanonicalSignatureMatchesClientCoreVector(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	meta := map[string]string{
		mirasimHeaderSession: "mirasim_00000000-0000-4000-8000-000000000000",
		mirasimHeaderAgent:   "claude",
		mirasimHeaderCall:    "11111111-2222-4333-8444-555555555555",
	}
	wantCanonical := "mrs-sig-v2\n" +
		"POST\n" +
		"/v1/messages\n" +
		"1788200000123\n" +
		"AAECAwQFBgcICQoL\n" +
		"device-fixed\n" +
		"0.0.260\n" +
		"66ee005427e4f3b74ce4830f104c989613f0968f97036191a6fbaea245040170\n" +
		"91bcb885e5b045a9f55f270bb0c6d633930407b6cca792839034702ed233be6b\n" +
		"9df27ddbfc24ebaafa990cd41a7744f56c875d0dadf69e4941edc7e728aea6bd"

	gotCanonical := mirasimCanonicalString("POST", "/v1/messages", 1788200000123,
		"AAECAwQFBgcICQoL", "device-fixed", "0.0.260", "ticket-fixed", meta,
		[]byte(`{"model":"claude-sonnet-5","messages":[]}`))
	if gotCanonical != wantCanonical {
		t.Fatalf("canonical payload 与官方向量不一致\n got: %q\nwant: %q", gotCanonical, wantCanonical)
	}

	hdrs := mirasimSignHeaders(priv, "device-fixed", mirasimSignParams{
		Method:        "POST",
		Path:          "/v1/messages",
		Credential:    "ticket-fixed",
		ClientVersion: "0.0.260",
		Meta:          meta,
		Body:          []byte(`{"model":"claude-sonnet-5","messages":[]}`),
	}, 1788200000123, "AAECAwQFBgcICQoL")
	wantSig := "zUYTEKW17Gzn7TEdEzWZ2aEOpO4oW9YFFpdsyzJaUyS4A_byq3DUNYzNOL96D24MExQ0mVbot75TkvkJw3vVAQ"
	if hdrs[mirasimHeaderSig] != wantSig {
		t.Fatalf("签名与官方向量不一致\n got: %q\nwant: %q", hdrs[mirasimHeaderSig], wantSig)
	}
}

// 官方 0.0.260 core 的确定性密封向量：相同的静态公钥、临时私钥、nonce、
// 明文与 AAD 必须得到同一个密文。
func TestMirasimSealPayloadMatchesClientCoreVector(t *testing.T) {
	recipientPublic, err := base64.StdEncoding.DecodeString("NYBy1jZYgNGu6jKa35EhODhR7SGijjt16WXQ0s0WYlQ=")
	if err != nil {
		t.Fatal(err)
	}
	ephemeralSecret, err := hex.DecodeString("404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f")
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := hex.DecodeString("a0a1a2a3a4a5a6a7a8a9aaab")
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"x-mirasim-agent":"claude","x-mirasim-call":"11111111-2222-4333-8444-555555555555","x-mirasim-device":"device-fixed","x-mirasim-nonce":"AAECAwQFBgcICQoL","x-mirasim-session":"mirasim_00000000-0000-4000-8000-000000000000","x-mirasim-sig":"zUYTEKW17Gzn7TEdEzWZ2aEOpO4oW9YFFpdsyzJaUyS4A_byq3DUNYzNOL96D24MExQ0mVbot75TkvkJw3vVAQ","x-mirasim-ts":"1788200000123"}`)
	aad := []byte("mrs-seal-v1\nPOST\n/v1/messages")

	sealed, err := mirasimSealPayload(recipientPublic, ephemeralSecret, nonce, plaintext, aad)
	if err != nil {
		t.Fatal(err)
	}
	want := "eaYx7t4b-cmPEgMs3q3Q56B5OY_HhriMyEbsia-FpRqgoaKjpKWmp6ipqqtWlxgybxeoS1fVDaS5_1az3V-kX_XGGNPghY8g8q81tF8LkfDoIwY8W2FWXoe5_27zjH9q2jM05ZuvNfmdYnjW0x616SP-p3g96-PvzI8GDuAbPgt9-0sIkQHeCCZ35opOpopxt_tdTp55bPp8CmjCpb1OR0aWs_5UezjAlVNibbN4979hGY_BcQ7z07Bkt92DCgJiP9aP8pLSXM1gcFHvnDDAiAqfqqA1cWx2f3EIHn585U-tdtQsRZ5BJ7wJ4sZgMswGl5CxDgSFJ-MhnsQsyj6zAR_MVujCO4jUkLVsRtI38N6sN-T79EWL4w4N1ksEfzIUJDtYDNfbr83XkXpl3sB6DvYIrrrPi5Gq96WSWldJf5Pgz0IdJq_O36wS2dNYVqQmsOU-nwgigBh0NrD94K23PthUn8qbkULkp7PgyPGDXN-4MkvdjN8LVN-oEP1oaP5FMVbiH6b_7K_9NaXvJCELj59p2P_fCnJ2ULHCpwyfDGOKjg"
	if got := base64.RawURLEncoding.EncodeToString(sealed); got != want {
		t.Fatalf("密封结果与官方向量不一致\n got: %q\nwant: %q", got, want)
	}
}

func TestMirasimSignHeadersVerify(t *testing.T) {
	priv, err := mirasimGenerateDeviceKey()
	if err != nil {
		t.Fatal(err)
	}
	deviceID, _ := mirasimDeviceIdentity(priv)
	body := []byte(`{"model":"claude-opus-4-8"}`)
	hdrs := mirasimSignHeaders(priv, deviceID, mirasimSignParams{
		Method:        "POST",
		Path:          "/v1/messages",
		Credential:    "ticket-xyz",
		ClientVersion: "0.0.322",
		Body:          body,
	}, 1758000000123, "")
	for _, k := range []string{mirasimHeaderDevice, mirasimHeaderTS, mirasimHeaderNonce, mirasimHeaderSig, mirasimHeaderClient} {
		if hdrs[k] == "" {
			t.Fatalf("缺少头 %s", k)
		}
	}
	var ts int64
	if _, err := fmt.Sscanf(hdrs[mirasimHeaderTS], "%d", &ts); err != nil {
		t.Fatal(err)
	}
	canonical := mirasimCanonicalString("POST", "/v1/messages", ts, hdrs[mirasimHeaderNonce], deviceID,
		"0.0.322", "ticket-xyz", nil, body)
	sig, err := base64.RawURLEncoding.DecodeString(hdrs[mirasimHeaderSig])
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	if !ed25519.Verify(pub, []byte(canonical), sig) {
		t.Fatal("ed25519 验签失败")
	}
	if n, err := base64.RawURLEncoding.DecodeString(hdrs[mirasimHeaderNonce]); err != nil || len(n) != 12 {
		t.Fatalf("nonce 非法: %v len=%d", err, len(n))
	}
}

// 模拟 relay 侧反向解封：断言布局、AAD 绑定与字段顺序。
func TestMirasimSealHeadersRoundTrip(t *testing.T) {
	relayPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	relayPubB64 := base64.StdEncoding.EncodeToString(relayPriv.PublicKey().Bytes())

	hdrs := map[string]string{
		mirasimHeaderDevice: "dev-123",
		mirasimHeaderTS:     "1758000000456",
		mirasimHeaderNonce:  "nonce-xyz",
		mirasimHeaderSig:    "sig-abc",
	}
	enc, err := mirasimSealHeaders(relayPubB64, "POST", "/v1/messages", hdrs)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 32+12+16 {
		t.Fatalf("seal 输出过短: %d", len(raw))
	}
	ephPubRaw, nonce, ctWithTag := raw[:32], raw[32:44], raw[44:]
	ct := ctWithTag[:len(ctWithTag)-16]
	if len(raw) != 32+12+len(ct)+16 {
		t.Fatalf("布局总长不符: %d != %d", len(raw), 32+12+len(ct)+16)
	}
	aead := mirasimRelaySideAEAD(t, relayPriv, ephPubRaw)
	pt, err := aead.Open(nil, nonce, ctWithTag, []byte("mrs-seal-v1\nPOST\n/v1/messages"))
	if err != nil {
		t.Fatalf("relay 侧解密失败: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(pt, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{mirasimHeaderDevice, mirasimHeaderTS, mirasimHeaderNonce, mirasimHeaderSig} {
		if got[k] != hdrs[k] {
			t.Fatalf("字段 %s 不符: %q != %q", k, got[k], hdrs[k])
		}
	}
	if len(got) != 4 {
		t.Fatalf("应只有 4 个字段: %v", got)
	}
	if _, err := aead.Open(nil, nonce, ctWithTag, []byte("mrs-seal-v1\nGET\n/v1/messages")); err == nil {
		t.Fatal("AAD 中 method 不匹配应解密失败")
	}
	if _, err := aead.Open(nil, nonce, ctWithTag, []byte("mrs-seal-v1\nPOST\n/v1/other")); err == nil {
		t.Fatal("AAD 中 path 不匹配应解密失败")
	}
}

func TestMirasimSealHeadersFieldOrder(t *testing.T) {
	relayPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	relayPubB64 := base64.StdEncoding.EncodeToString(relayPriv.PublicKey().Bytes())
	enc, err := mirasimSealHeaders(relayPubB64, "GET", "/v1/models", map[string]string{
		mirasimHeaderDevice: "d", mirasimHeaderTS: "1", mirasimHeaderNonce: "n", mirasimHeaderSig: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatal(err)
	}
	ephPubRaw, nonce, ctWithTag := raw[:32], raw[32:44], raw[44:]
	aead := mirasimRelaySideAEAD(t, relayPriv, ephPubRaw)
	pt, err := aead.Open(nil, nonce, ctWithTag, []byte("mrs-seal-v1\nGET\n/v1/models"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"x-mirasim-device":"d","x-mirasim-nonce":"n","x-mirasim-sig":"s","x-mirasim-ts":"1"}`
	if string(pt) != want {
		t.Fatalf("plaintext 字段顺序不符:\n%s\nwant:\n%s", pt, want)
	}
}

func TestMirasimSealHeadersRejectsBadPubkey(t *testing.T) {
	if _, err := mirasimSealHeaders("not-base64!!", "POST", "/v1/messages", nil); err == nil {
		t.Fatal("非法 seal 公钥应报错")
	}
	if _, err := mirasimSealHeaders(base64.StdEncoding.EncodeToString([]byte("short")), "POST", "/v1/messages", nil); err == nil {
		t.Fatal("长度非法的 seal 公钥应报错")
	}
}

func mirasimRelaySideAEAD(t *testing.T, relayPriv *ecdh.PrivateKey, ephPubRaw []byte) interface {
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
} {
	t.Helper()
	ephPub, err := ecdh.X25519().NewPublicKey(ephPubRaw)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := relayPriv.ECDH(ephPub)
	if err != nil {
		t.Fatal(err)
	}
	hk := hkdf.New(sha256.New, shared, ephPubRaw, []byte(mirasimSealInfo))
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hk, key); err != nil {
		t.Fatal(err)
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return aead
}
