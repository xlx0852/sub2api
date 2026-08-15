.PHONY: build build-backend build-frontend build-embedded test test-backend test-frontend test-frontend-critical

FRONTEND_CRITICAL_VITEST := \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts

# 默认分别构建纯 API 与独立前端；两个产物可以独立部署。
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 兼容交付：先构建前端，再显式打入单二进制。
build-embedded: build-frontend
	@$(MAKE) -C backend build LDFLAGS="-s -w -X main.Version=$$(tr -d '\r\n' < backend/cmd/server/VERSION) -X main.MinFrontendVersion=$$(tr -d '\r\n' < backend/cmd/server/MIN_FRONTEND_VERSION)" \
		GOFLAGS="-tags=embed"

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run check:api
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)
