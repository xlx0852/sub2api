package service

// Mirasim relay 协议原语（mrs-sig-v2 签名 + mrs-seal-v1 密封 + Ed25519 设备身份）。
//
// 协议来自参考实现 github.com/Essaim8/mirasim2api 的 internal/mirasim/crypto.go，
// 本文件与其逐字节等价；crypto_test.go 中的钉住向量被搬到 mirasim_protocol_test.go
// 作为回归基准。任何改动都必须先让这些向量继续通过。
//
// 关键约束（改动前请先读）：
//   - canonical string 共 10 行，meta 为空时第 9 行是**空行**（不是空串哈希）；
//   - 密封 AAD 与签名 canonical 都含请求 path，因此实际发出的 path 必须与
//     签名时使用的 path 完全一致；
//   - 密封明文是固定字段顺序的 4 字段 JSON，不能用 map（字典序会变）。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	mirasimSigVersion   = "mrs-sig-v2"
	mirasimSealVersion  = "mrs-seal-v1"
	mirasimSealInfo     = "mrs-seal-v1"
	mirasimMRS1Prefix   = "mrs1:"
	mirasimNonceSize    = 12
	mirasimDeviceIDSize = 22

	// Mirasim 协议头名。
	mirasimHeaderDevice = "x-mirasim-device"
	mirasimHeaderTS     = "x-mirasim-ts"
	mirasimHeaderNonce  = "x-mirasim-nonce"
	mirasimHeaderSig    = "x-mirasim-sig"
	mirasimHeaderClient = "x-mirasim-client"
	mirasimHeaderEnc    = "x-mirasim-enc"

	// relay 元数据头：官方客户端每次推理都带，且密封进 x-mirasim-enc。
	mirasimHeaderSession = "x-mirasim-session"
	mirasimHeaderAgent   = "x-mirasim-agent"
	mirasimHeaderCall    = "x-mirasim-call"
	mirasimHeaderLocale  = "x-mirasim-locale"
	mirasimHeaderCollect = "x-mirasim-collect"
)

func mirasimSHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// mirasimRandomUUID 生成 RFC 4122 v4 UUID（与官方客户端同格式）。
// 用于 x-mirasim-call（每次上游尝试一个新 id）与本地稳定 session 回退值。
func mirasimRandomUUID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Sprintf("mirasim: uuid 生成失败: %v", err))
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

// mirasimCanonicalString 构造 mrs-sig-v2 签名原文。
//
// 10 行以 "\n" 连接：
//
//	mrs-sig-v2
//	UPPER(method)
//	path
//	ts(ms)
//	nonce
//	deviceID
//	clientVersion
//	sha256hex(credential)
//	metaLine
//	sha256hex(body)
//
// 关键约束（改动前请先读）：metaLine 必须与**官方客户端 0.0.260 crypto core** 逐字节一致：
// 键小写、丢空值、键排序、每条写成 `key:value`、以 "\n" 连接，再整体 sha256；
// 元数据为空时第 9 行是**空行**（不是空串的 sha256）。
// 早先实现用的是 `k\0v\0` 拼接，只因我们一直发空元数据才没暴露；
// 一旦携带 session/agent/call，那个实现会被上游判签名无效。
func mirasimCanonicalString(method, path string, ts int64, nonce, deviceID, clientVersion, credential string, meta map[string]string, body []byte) string {
	metaLine := ""
	if len(meta) > 0 {
		normalized := make(map[string]string, len(meta))
		for k, v := range meta {
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "" || v == "" {
				continue
			}
			normalized[k] = v
		}
		if len(normalized) > 0 {
			keys := make([]string, 0, len(normalized))
			for k := range normalized {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			pairs := make([]string, 0, len(keys))
			for _, k := range keys {
				pairs = append(pairs, k+":"+normalized[k])
			}
			metaLine = mirasimSHA256Hex([]byte(strings.Join(pairs, "\n")))
		}
	}
	lines := []string{
		mirasimSigVersion,
		strings.ToUpper(method),
		path,
		strconv.FormatInt(ts, 10),
		nonce,
		deviceID,
		clientVersion,
		mirasimSHA256Hex([]byte(credential)),
		metaLine,
		mirasimSHA256Hex(body),
	}
	return strings.Join(lines, "\n")
}

// mirasimSignParams 是 mirasimSignHeaders 的入参。
type mirasimSignParams struct {
	Method        string
	Path          string
	Credential    string // /v1/device/session 用 access token；模型路由用 ticket
	ClientVersion string
	Meta          map[string]string
	Body          []byte
}

// mirasimSignHeaders 生成 5 个明文签名头（只有 /v1/device/session 这样发）。
// 时间戳与 nonce 由参数显式传入以支持单测固定向量。
func mirasimSignHeaders(priv ed25519.PrivateKey, deviceID string, p mirasimSignParams, ts int64, nonce string) map[string]string {
	if nonce == "" {
		raw := make([]byte, mirasimNonceSize)
		if _, err := rand.Read(raw); err != nil {
			panic(fmt.Sprintf("mirasim: nonce 生成失败: %v", err))
		}
		nonce = base64.RawURLEncoding.EncodeToString(raw)
	}
	canonical := mirasimCanonicalString(p.Method, p.Path, ts, nonce, deviceID, p.ClientVersion, p.Credential, p.Meta, p.Body)
	sig := ed25519.Sign(priv, []byte(canonical))
	return map[string]string{
		mirasimHeaderDevice: deviceID,
		mirasimHeaderTS:     strconv.FormatInt(ts, 10),
		mirasimHeaderNonce:  nonce,
		mirasimHeaderSig:    base64.RawURLEncoding.EncodeToString(sig),
		mirasimHeaderClient: p.ClientVersion,
	}
}

// mirasimSealPlaintext 按参考实现的字段顺序序列化（struct 而非 map，保证字节一致）。
//
// 已被 mirasimSealableHeaders 取代：官方客户端把**所有**待密封头（签名头 + relay
// 元数据头）放进同一个 JSON 对象，键按字典序，因此保留类型仅为兼容旧引用。
type mirasimSealPlaintext struct {
	Device string `json:"x-mirasim-device"`
	TS     string `json:"x-mirasim-ts"`
	Nonce  string `json:"x-mirasim-nonce"`
	Sig    string `json:"x-mirasim-sig"`
}

// mirasimIsSealedHeader 判定一个（小写）头名是否应进入 x-mirasim-enc。
//
// 关键约束（改动前请先读）：官方客户端密封**除 x-mirasim-client 与
// x-mirasim-enc 之外的所有 x-mirasim-* 头**，因此 relay 元数据
// （session/agent/call/account/locale/collect）也进密封块，明文线上不应出现。
func mirasimIsSealedHeader(lowerName string) bool {
	return strings.HasPrefix(lowerName, "x-mirasim-") &&
		lowerName != mirasimHeaderClient &&
		lowerName != mirasimHeaderEnc
}

// mirasimSealableHeaders 收集待密封头（键转小写、丢空值），返回键排序后的副本。
// 无任何待密封头时返回 nil（调用方不应设置 x-mirasim-enc）。
func mirasimSealableHeaders(headers map[string]string) map[string]string {
	sealed := make(map[string]string, len(headers))
	for name, value := range headers {
		lower := strings.ToLower(strings.TrimSpace(name))
		if value == "" || !mirasimIsSealedHeader(lower) {
			continue
		}
		sealed[lower] = value
	}
	if len(sealed) == 0 {
		return nil
	}
	return sealed
}

// mirasimSealHeaders 实现 mrs-seal-v1：把**所有待密封头**密封进 x-mirasim-enc。
//
//	shared = ECDH(X25519 临时私钥, relay 静态公钥)
//	key    = HKDF-SHA256(ikm=shared, salt=临时公钥, info="mrs-seal-v1", L=32)
//	AAD    = "mrs-seal-v1\n" + UPPER(method) + "\n" + path
//	明文    = 待密封头的 JSON 对象（键按字典序；含签名头与 relay 元数据头）
//	输出    = base64url(临时公钥(32) || nonce(12) || 密文 || tag(16))
//
// 返回空串表示没有任何待密封头（调用方不得设置 x-mirasim-enc）。
func mirasimSealHeaders(relayPubkeyB64, method, path string, headers map[string]string) (string, error) {
	recipient, err := mirasimDecodeSealPubkey(relayPubkeyB64)
	if err != nil {
		return "", err
	}
	sealed := mirasimSealableHeaders(headers)
	if len(sealed) == 0 {
		return "", nil
	}
	ephSecret := make([]byte, 32)
	if _, err := rand.Read(ephSecret); err != nil {
		return "", fmt.Errorf("mirasim: 临时密钥生成失败: %w", err)
	}
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("mirasim: seal nonce 生成失败: %w", err)
	}
	pt, err := json.Marshal(sealed)
	if err != nil {
		return "", fmt.Errorf("mirasim: 密封明文序列化失败: %w", err)
	}
	aad := []byte(mirasimSealVersion + "\n" + strings.ToUpper(method) + "\n" + path)
	out, err := mirasimSealPayload(recipient, ephSecret, nonce, pt, aad)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// mirasimDecodeSealPubkey 解析并校验 relay 静态公钥（32 字节 X25519）。
// 即使本次没有待密封头也先校验：配置写错时应尽早报错，而不是静默降级。
func mirasimDecodeSealPubkey(relayPubkeyB64 string) ([]byte, error) {
	relayPubRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(relayPubkeyB64))
	if err != nil {
		return nil, fmt.Errorf("mirasim: seal 公钥 base64 解码失败: %w", err)
	}
	if len(relayPubRaw) != 32 {
		return nil, fmt.Errorf("mirasim: seal 公钥必须为 32 字节，实际 %d", len(relayPubRaw))
	}
	return relayPubRaw, nil
}

// mirasimSealPayload 是 mrs-seal-v1 的纯函数部分（显式接收临时私钥与 nonce），
// 便于用官方 0.0.260 crypto core 的钉住向量做逐字节回归。
func mirasimSealPayload(relayPubRaw, ephemeralSecret, nonce, plaintext, aad []byte) ([]byte, error) {
	if len(relayPubRaw) != 32 {
		return nil, fmt.Errorf("mirasim: seal 公钥必须为 32 字节")
	}
	if len(ephemeralSecret) != 32 {
		return nil, fmt.Errorf("mirasim: 临时密钥必须为 32 字节")
	}
	if len(nonce) != chacha20poly1305.NonceSize {
		return nil, fmt.Errorf("mirasim: seal nonce 必须为 12 字节")
	}
	relayPub, err := ecdh.X25519().NewPublicKey(relayPubRaw)
	if err != nil {
		return nil, fmt.Errorf("mirasim: seal 公钥非法: %w", err)
	}
	ephPriv, err := ecdh.X25519().NewPrivateKey(ephemeralSecret)
	if err != nil {
		return nil, fmt.Errorf("mirasim: 临时密钥非法: %w", err)
	}
	shared, err := ephPriv.ECDH(relayPub)
	if err != nil {
		return nil, fmt.Errorf("mirasim: ECDH 失败: %w", err)
	}
	ephPub := ephPriv.PublicKey().Bytes()
	hk := hkdf.New(sha256.New, shared, ephPub, []byte(mirasimSealInfo))
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hk, key); err != nil {
		return nil, fmt.Errorf("mirasim: HKDF 派生失败: %w", err)
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("mirasim: AEAD 初始化失败: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, aad) // ct||tag
	out := make([]byte, 0, len(ephPub)+len(nonce)+len(sealed))
	out = append(out, ephPub...)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// mirasimGenerateDeviceKey 生成全新的 Ed25519 设备密钥。
func mirasimGenerateDeviceKey() (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("mirasim: 设备密钥生成失败: %w", err)
	}
	return priv, nil
}

// mirasimDeviceIdentity 派生设备身份：
//
//	publicKeyB64 = base64(SPKI/PKIX DER)
//	deviceID     = base64url(sha256(publicKeyB64 的 ASCII 字节))[:22]
func mirasimDeviceIdentity(priv ed25519.PrivateKey) (deviceID, publicKeyB64 string) {
	pub := priv.Public().(ed25519.PublicKey)
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		// Ed25519 公钥的 PKIX 序列化不会失败，这里仅作防御。
		panic(fmt.Sprintf("mirasim: 公钥序列化失败: %v", err))
	}
	publicKeyB64 = base64.StdEncoding.EncodeToString(der)
	sum := sha256.Sum256([]byte(publicKeyB64))
	deviceID = base64.RawURLEncoding.EncodeToString(sum[:])[:mirasimDeviceIDSize]
	return deviceID, publicKeyB64
}

// mirasimMarshalPrivateKeyPEM 序列化 Ed25519 私钥为 PKCS8 PEM。
func mirasimMarshalPrivateKeyPEM(priv ed25519.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("mirasim: 私钥序列化失败: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// mirasimParsePrivateKeyPEM 解析 PKCS8 PEM 的 Ed25519 私钥。
func mirasimParsePrivateKeyPEM(pemStr string) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("mirasim: 设备私钥 PEM 解码失败")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("mirasim: 设备私钥解析失败: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("mirasim: 非 Ed25519 设备私钥: %T", key)
	}
	return priv, nil
}

// mirasimEncryptMRS1 用 AES-256-GCM 加密为 "mrs1:" + base64(iv(12)||tag(16)||ct)。
// 保留此原语用于本地密文存储（如未来需要加密保存 refresh token）。
func mirasimEncryptMRS1(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("mirasim: 非法 master key 长度 %d", len(key))
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("mirasim: GCM 初始化失败: %w", err)
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("mirasim: IV 生成失败: %w", err)
	}
	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)
	ct, tag := sealed[:len(sealed)-gcm.Overhead()], sealed[len(sealed)-gcm.Overhead():]
	out := make([]byte, 0, len(iv)+len(tag)+len(ct))
	out = append(out, iv...)
	out = append(out, tag...)
	out = append(out, ct...)
	return mirasimMRS1Prefix + base64.StdEncoding.EncodeToString(out), nil
}

// mirasimDecryptMRS1 解密 mrs1 blob；非 "mrs1:" 前缀的输入原样返回。
func mirasimDecryptMRS1(blob string, key []byte) (string, error) {
	if !strings.HasPrefix(blob, mirasimMRS1Prefix) {
		return blob, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(blob, mirasimMRS1Prefix))
	if err != nil {
		return "", fmt.Errorf("mirasim: mrs1 base64 解码失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("mirasim: 非法 master key 长度 %d", len(key))
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("mirasim: GCM 初始化失败: %w", err)
	}
	ns, ov := gcm.NonceSize(), gcm.Overhead()
	if len(raw) < ns+ov {
		return "", errors.New("mirasim: mrs1 blob 过短")
	}
	iv, tag, ct := raw[:ns], raw[ns:ns+ov], raw[ns+ov:]
	sealed := append(append([]byte{}, ct...), tag...)
	pt, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("mirasim: mrs1 解密失败: %w", err)
	}
	return string(pt), nil
}
