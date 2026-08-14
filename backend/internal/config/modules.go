package config

// 以下访问器让模块只取自己的子结构（design.md D7），YAML/env 键名不变。

func (c *Config) GatewayModule() GatewayConfig {
	if c == nil {
		return GatewayConfig{}
	}
	return c.Gateway
}

func (c *Config) BillingModule() BillingConfig {
	if c == nil {
		return BillingConfig{}
	}
	return c.Billing
}

func (c *Config) SecurityModule() SecurityConfig {
	if c == nil {
		return SecurityConfig{}
	}
	return c.Security
}

func (c *Config) DatabaseModule() DatabaseConfig {
	if c == nil {
		return DatabaseConfig{}
	}
	return c.Database
}

func (c *Config) RedisModule() RedisConfig {
	if c == nil {
		return RedisConfig{}
	}
	return c.Redis
}

func (c *Config) OpsModule() OpsConfig {
	if c == nil {
		return OpsConfig{}
	}
	return c.Ops
}

func (c *Config) ServerModule() ServerConfig {
	if c == nil {
		return ServerConfig{}
	}
	return c.Server
}
