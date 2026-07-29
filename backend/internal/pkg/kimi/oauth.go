package kimi

const (
	ClientID                    = "17e5f671-d194-4dfb-9706-5516cb48c098"
	OAuthBaseURL                = "https://auth.kimi.com"
	DeviceAuthorizationURL      = OAuthBaseURL + "/api/oauth/device_authorization"
	TokenURL                    = OAuthBaseURL + "/api/oauth/token"
	CodingBaseURL               = "https://api.kimi.com/coding"
	DeviceCodeGrantType         = "urn:ietf:params:oauth:grant-type:device_code"
	DefaultDevicePollInterval   = 5
	DefaultDeviceCodeExpiresIn  = 900
	DefaultAccessTokenExpiresIn = 21600
	ClientUserAgent             = "KimiCLI/1.10.6"
	ClientPlatform              = "kimi_cli"
	ClientVersion               = "1.10.6"
)

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}

type TokenResponse struct {
	AccessToken      string  `json:"access_token"`
	RefreshToken     string  `json:"refresh_token"`
	TokenType        string  `json:"token_type"`
	Scope            string  `json:"scope"`
	ExpiresIn        float64 `json:"expires_in"`
	Error            string  `json:"error"`
	ErrorDescription string  `json:"error_description"`
}

type DeviceHeaders struct {
	DeviceID    string
	DeviceName  string
	DeviceModel string
}
