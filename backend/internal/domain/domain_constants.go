// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

// SubscriptionStatusRevoked 是 soft-deleted 订阅的 API 展示态，不写入 status 字段。
const SubscriptionStatusRevoked = "revoked"

// LinuxDoConnectSyntheticEmailDomain 是 LinuxDo Connect 用户的合成邮箱后缀（RFC 保留域名）。
const LinuxDoConnectSyntheticEmailDomain = "@linuxdo-connect.invalid"

// OIDCConnectSyntheticEmailDomain 是 OIDC 用户的合成邮箱后缀（RFC 保留域名）。
const OIDCConnectSyntheticEmailDomain = "@oidc-connect.invalid"

// WeChatConnectSyntheticEmailDomain 是 WeChat Connect 用户的合成邮箱后缀（RFC 保留域名）。
const WeChatConnectSyntheticEmailDomain = "@wechat-connect.invalid"

// DingTalkConnectSyntheticEmailDomain 是 DingTalk Connect 用户的合成邮箱后缀（RFC 保留域名）。
const DingTalkConnectSyntheticEmailDomain = "@dingtalk-connect.invalid"
