# Change: 优化 Profit 配额时间轴渲染

## Why

Profit 配额甘特当前为每个账号重复创建全部可见刻度节点，并在每个原生滚动事件中重新解析日期、扩展窗口与放大时间轴。账号和历史窗口增多后，DOM 数量、计算量和时间轴宽度都会持续增长，造成滚动掉帧。

## What Changes

- 将行内重复刻度合并为跨全部行的一层共享网格。
- 将窗口日期预解析与投影计算拆分，只生成视口及过扫描范围内的窗口。
- 将无限扩展时间轴改为固定容量的无缝换基准时间轴，长期滚动时宽度和节点数保持稳定。
- 将滚动状态更新合并到动画帧，并缓存本地化日期格式器。
- 保持真实账本窗口、投影兜底、待激活空档、订阅周期裁剪和抽屉选择合同不变。

## Impact

- Affected specs: `account-profit-quota-windows`
- Affected code: `frontend/src/components/admin/profit/QuotaWindowPanel.vue`, timeline calculation helpers and component tests.

