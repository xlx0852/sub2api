## 1. Implementation
- [x] 1.1 删除管理端版本徽标中的远端更新检测、更新提示与在线更新交互，仅保留本地版本展示
  - 备注：徽标已只展示 `siteVersion`（来自本地构建/公开配置）和重启；不请求 GitHub。
- [x] 1.2 清理前端与版本更新检查相关的状态、API 封装和测试桩
  - 备注：删除未使用的「最新版本/立即更新」i18n；GitHubReleaseClient 仍留给 Codex 版本同步与定价拉取，不作为管理端在线更新。
- [x] 1.3 调整系统管理接口，使版本接口只返回当前构建版本，并移除更新检查 / 在线更新 / 回滚路由
  - 备注：`GET /admin/system/version` 只返回 `version`；路由护栏断言无 update/rollback 入口。
- [x] 1.4 清理 GitHub Release 更新服务及依赖注入，补齐受影响测试
  - 备注：管理端更新服务已不存在；保留 `github_release_service` 给 Codex 同步。补 VersionBadge / GetVersion / 路由回归。
