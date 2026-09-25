package service

// 上游请求体 gzip 压缩：第三方 OpenAI 兼容中转在收大请求体（compact / 长上下文
// 的 /responses）时，出站写入耗时可以达到数百毫秒到数秒（实测 body p50 ≈ 5.8MB、
// upstream_write_ms p50 ≈ 495ms / p95 ≈ 2.5s）。压缩后写出字节量通常降到 1/5 ~ 1/10。
//
// 关键约束（改动前请先读）：
//   - 只对 OpenAIGatewayService 的 **APIKey 账号**生效；OAuth(Codex) 走 zstd（见
//     openai_codex_fingerprint.go），外部桥（Devin/Cursor 等自建 sidecar）与 compact
//     unary 协议未验证，一律不压；
//   - 上游白名单来自配置（host 后缀匹配）；不在白名单时保持原样，宁可不压也不冒失败风险；
//   - 体积低于阈值时不压（压缩本身有成本，小体量无收益）；
//   - 压缩失败时返回原体与 false，绝不把压缩变成新的失败点。
//   - 兜底：若上游因编码返回 4xx，运维可通过清空白名单配置即时回退，无需重新发布。

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

// maybeGzipOpenAIUpstreamBody 按配置对出站请求体做 gzip 压缩。
// 返回 true 表示已压缩并已改写 req（Body / ContentLength / GetBody / Content-Encoding）。
func maybeGzipOpenAIUpstreamBody(req *http.Request, body []byte, account *Account, cfg *config.Config) bool {
	if req == nil || account == nil || cfg == nil || len(body) == 0 {
		return false
	}
	// 先记录未压缩的出站长度，供瀑布图/导出对照；压缩成功时再覆盖为压缩后长度。
	if collector := requesttiming.FromContext(req.Context()); collector != nil {
		collector.SetUpstreamRequestBytes(int64(len(body)))
	}
	settings := cfg.Gateway
	if settings.UpstreamGzipMinBytes <= 0 || len(settings.UpstreamGzipHosts) == 0 {
		return false
	}
	if int64(len(body)) < settings.UpstreamGzipMinBytes {
		return false
	}
	if account.Type != AccountTypeAPIKey || account.IsExternalBridge() || !account.IsOpenAI() {
		return false
	}
	if !upstreamGzipHostAllowed(account.GetOpenAIBaseURL(), settings.UpstreamGzipHosts) {
		return false
	}
	compressed, err := gzipBody(body)
	if err != nil || len(compressed) >= len(body) {
		return false
	}
	req.Body = io.NopCloser(bytes.NewReader(compressed))
	req.ContentLength = int64(len(compressed))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(compressed)), nil
	}
	req.Header.Set(openAIRequestCompressionHeader, "gzip")
	if collector := requesttiming.FromContext(req.Context()); collector != nil {
		collector.SetUpstreamRequestBytes(int64(len(compressed)))
	}
	return true
}

// upstreamGzipHostAllowed 按 host 后缀匹配白名单；base_url 为空或非法时返回 false。
func upstreamGzipHostAllowed(baseURL string, allowed []string) bool {
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

func gzipBody(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
