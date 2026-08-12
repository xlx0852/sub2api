## 1. Backend runtime contract

- [x] 1.1 删除 `RunModeSimple` 常量和所有简洁模式分支，仅允许标准模式
- [x] 1.2 对环境变量和 YAML 中的 `simple` 值返回带迁移说明的启动错误
- [x] 1.3 删除认证、计费、配额、调度、清理和定时任务中的简洁模式旁路
- [x] 1.4 删除简洁模式默认分组、管理员并发升级和 setup 初始化逻辑
- [x] 1.5 从认证/API 契约中删除 `run_mode`

## 2. Frontend removal

- [x] 2.1 删除 auth store 和 API 类型中的 `run_mode` / `isSimpleMode`
- [x] 2.2 删除路由守卫、侧边栏过滤和简洁模式专用入口排序
- [x] 2.3 删除页面、账号弹窗、仪表盘和引导中的简洁模式分支
- [x] 2.4 更新对应单元测试和集成测试

## 3. Documentation and migration

- [x] 3.1 更新 README 和部署配置示例，删除启用简洁模式的说明
- [x] 3.2 增加从简洁模式迁移到标准模式的检查清单
- [x] 3.3 更新 API 契约测试和配置校验测试

## 4. Verification

- [x] 4.1 运行后端相关单测和全包编译
- [x] 4.2 运行前端测试、类型检查和生产构建
- [x] 4.3 全仓搜索确认生产代码、文档和配置不再暴露简洁模式
- [x] 4.4 运行 `openspec validate remove-simple-run-mode --strict`
