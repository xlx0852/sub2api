package repository

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"

	"github.com/imroc/req/v3"
)

// reqClientOptions 定义 req 客户端的构建参数
type reqClientOptions struct {
	ProxyURL    string        // 代理 URL（支持 http/https/socks5）
	Timeout     time.Duration // 请求超时时间
	Impersonate bool          // 是否模拟 Chrome 浏览器指纹
	ForceHTTP2  bool          // 是否强制使用 HTTP/2
}

// ReqClientPool 按配置参数缓存 req.Client，复用底层连接池。
// 由 Wire 注入同一实例给各 OAuth client 与 PrivacyClientFactory。
type ReqClientPool struct {
	clients sync.Map
}

// NewReqClientPool 创建空的共享 req 客户端池。
func NewReqClientPool() *ReqClientPool {
	return &ReqClientPool{}
}

func orNewReqClientPool(pool *ReqClientPool) *ReqClientPool {
	if pool != nil {
		return pool
	}
	return NewReqClientPool()
}

// get 获取共享的 req 客户端。nil 接收者每次新建、不缓存，供测试零值构造。
func (p *ReqClientPool) get(opts reqClientOptions) (*req.Client, error) {
	if p == nil {
		return newReqClient(opts)
	}
	key := buildReqClientKey(opts)
	if cached, ok := p.clients.Load(key); ok {
		if c, ok := cached.(*req.Client); ok {
			return c, nil
		}
	}

	client, err := newReqClient(opts)
	if err != nil {
		return nil, err
	}

	actual, _ := p.clients.LoadOrStore(key, client)
	if c, ok := actual.(*req.Client); ok {
		return c, nil
	}
	return client, nil
}

func newReqClient(opts reqClientOptions) (*req.Client, error) {
	client := req.C().SetTimeout(opts.Timeout)
	if opts.ForceHTTP2 {
		client = client.EnableForceHTTP2()
	}
	if opts.Impersonate {
		client = client.ImpersonateChrome()
	}
	trimmed, _, err := proxyurl.Parse(opts.ProxyURL)
	if err != nil {
		return nil, err
	}
	if trimmed != "" {
		client.SetProxyURL(trimmed)
	}
	return client, nil
}

func buildReqClientKey(opts reqClientOptions) string {
	return fmt.Sprintf("%s|%s|%t|%t",
		strings.TrimSpace(opts.ProxyURL),
		opts.Timeout.String(),
		opts.Impersonate,
		opts.ForceHTTP2,
	)
}

// CreatePrivacyReqClient creates an HTTP client for OpenAI privacy settings API.
// Uses Chrome TLS fingerprint impersonation to bypass Cloudflare checks.
func (p *ReqClientPool) CreatePrivacyReqClient(proxyURL string) (*req.Client, error) {
	return p.get(reqClientOptions{
		ProxyURL:    proxyURL,
		Timeout:     30 * time.Second,
		Impersonate: true,
	})
}

func createOpenAIReqClient(pool *ReqClientPool, proxyURL string) (*req.Client, error) {
	return pool.get(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  120 * time.Second,
	})
}

func createGeminiReqClient(pool *ReqClientPool, proxyURL string) (*req.Client, error) {
	return pool.get(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}

func createKimiReqClient(pool *ReqClientPool, proxyURL string) (*req.Client, error) {
	return pool.get(reqClientOptions{ProxyURL: proxyURL, Timeout: 60 * time.Second})
}

func createGrokReqClient(pool *ReqClientPool, proxyURL string) (*req.Client, error) {
	return pool.get(reqClientOptions{
		ProxyURL: proxyURL,
		Timeout:  60 * time.Second,
	})
}
