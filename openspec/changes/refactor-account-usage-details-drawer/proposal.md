# Change: 重构账号用量摘要与详情抽屉

## Why

账号管理当前把上游额度窗口、本地消耗、费用、刷新操作和诊断状态集中在固定宽度的表格单元格中。随着 OpenAI、Claude、Grok、Gemini 等平台展示口径增加，列表难以快速扫描，各平台的操作文案和信息层级也不一致。

## What Changes

- 将账号列表的“用量窗口”列收敛为统一摘要，只展示最重要的额度窗口、今日消耗和风险状态
- 点击摘要后从右侧打开账号用量详情抽屉
- 抽屉按“额度、消耗、性能、诊断”四类信息组织现有数据
- 统一“刷新额度、刷新统计、查看详情”等操作文案和语义
- 保留不同平台的额度模型差异，但使用统一的尺寸、间距、状态颜色和空态
- 移动端将右侧抽屉适配为全宽详情层
- 不改变后端统计接口、额度探测、计费口径和账号调度逻辑

## Impact

- Affected specs: `account-usage-presentation`
- Affected code: `AccountsView.vue`、`AccountUsageCell.vue`、账号统计弹窗、用量进度和操作组件、账号用量相关 i18n 与前端测试
- Backend/API impact: none
