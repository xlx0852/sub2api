package service

// 上游请求体压缩：第三方 OpenAI 兼容中转在收大请求体（长上下文 /responses）时，
// 出站写入耗时随字节量上升（实测 100–170ms/出站 MiB + 250–300ms 固定开销）。
//
// 编码选择（2026-09-25 实测 o10.top 对 gzip 与 zstd 均返回 200）：
//   - 命中 zstd 白名单 → zstd（Codex 官方客户端对所有 /responses 请求体就用它，压缩率优于 gzip）；
//   - 仅命中 gzip 白名单 → gzip（DefaultCompression，比 BestSpeed 更省字节）；
//   - 都不命中 / 体积低于阈值 / 非 APIKey 账号 / 外部桥 → 不压缩。
//
// 关键约束（改动前请先读）：
//   - OAuth(Codex) 走既有 zstd 分支（openai_codex_fingerprint.go），本文件只管非 OAuth；
//   - 白名单来自配置，清空即回退，无需重新发布；启动日志打印生效值；
//   - 压缩失败返回原体，绝不把压缩变成新的失败点。

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requesttiming"
)

// maybeCompressOpenAIUpstreamBody 按配置对出站请求体做压缩。
// 返回 true 表示已压缩并改写 req（Body / ContentLength / GetBody / Content-Encoding）。
func maybeCompressOpenAIUpstreamBody(req *http.Request, body []byte, account *Account, cfg *config.Config) bool {
	if req == nil || account == nil || cfg == nil || len(body) == 0 {
		return false
	}
	settings := cfg.Gateway
	if settings.UpstreamGzipMinBytes <= 0 || len(body) < int(settings.UpstreamGzipMinBytes) {
		return false
	}
	if account.Type != AccountTypeAPIKey || account.IsExternalBridge() || !account.IsOpenAI() {
		return false
	}
	baseURL := account.GetOpenAIBaseURL()
	codec := ""
	switch {
	case upstreamHostAllowed(baseURL, settings.UpstreamZstdHosts):
		codec = "zstd"
	case upstreamHostAllowed(baseURL, settings.UpstreamGzipHosts):
		codec = "gzip"
	default:
		return false
	}

	var compressed []byte
	switch codec {
	case "zstd":
		compressed = encodeUpstreamZstd(body)
	case "gzip":
		compressed = encodeUpstreamGzip(body)
	}
	if len(compressed) == 0 || len(compressed) >= len(body) {
		return false
	}
	req.Body = io.NopCloser(bytes.NewReader(compressed))
	req.ContentLength = int64(len(compressed))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(compressed)), nil
	}
	req.Header.Set(openAIRequestCompressionHeader, codec)
	if collector := requesttiming.FromContext(req.Context()); collector != nil {
		collector.SetUpstreamRequestBytes(int64(len(compressed)))
	}
	return true
}

// encodeUpstreamZstd 复用进程级 zstd 编码器（并发 EncodeAll 安全）。
func encodeUpstreamZstd(body []byte) []byte {
	if openAIUpstreamZstdEncoder == nil {
		return nil
	}
	compressed := openAIUpstreamZstdEncoder.EncodeAll(body, make([]byte, 0, len(body)/2))
	if len(compressed) == 0 {
		return nil
	}
	return compressed
}

func encodeUpstreamGzip(body []byte) []byte {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.DefaultCompression)
	if err != nil {
		return nil
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return nil
	}
	if err := writer.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

// upstreamHostAllowed 按 host 后缀匹配白名单；base_url 为空或非法时返回 false。
func upstreamHostAllowed(baseURL string, allowed []string) bool {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return false
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, candidate := range allowed {
		pattern := strings.ToLower(strings.TrimSpace(candidate))
		if pattern == "" {
			continue
		}
		if host == pattern || strings.HasSuffix(host, "."+pattern) {
			return true
		}
	}
	return false
}

// recordUpstreamBodyBytesIfUncompressed 在决定不压缩时也记录出站长度，
// 便于「出站/入站」对照观测。
func recordUpstreamBodyBytesIfUncompressed(req *http.Request, body []byte) {
	if req == nil || len(body) == 0 {
		return
	}
	if req.Header.Get(openAIRequestCompressionHeader) != "" {
		return
	}
	if collector := requesttiming.FromContext(req.Context()); collector != nil {
		collector.SetUpstreamRequestBytes(int64(len(body)))
	}
}
