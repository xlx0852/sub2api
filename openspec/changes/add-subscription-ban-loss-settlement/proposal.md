# Change: 新增订阅账号封禁亏损结算

## Why

订阅账号被 OpenAI 等上游封禁后会立即失去后续产出能力，但当前系统仍会把整笔采购费用按原计划周期继续摊销，也无法记录不退款或部分退款带来的实际亏损。这会让账号回本进度和全局利润在封禁后仍虚假地继续变化。

## What Changes

- 在订阅周期上新增显式的“封禁结算”操作，记录封禁生效时间、原因和备注，不从通用错误、限流或暂停状态自动推断财务事实。
- 封禁结算成功时，在同一事务中把对应账号及共享其凭据的影子账号设为 `disabled` 且 `schedulable=false`，并通知调度快照立即移出供给。
- 新增可追加的实际退款记录：初始可为 0，以后收到部分退款时按实际到账日冲减成本，不把“申请中”或“预计退款”当成已收到。
- 账号抽屉和成本弹窗展示封禁时点、封禁前收入、已到账退款、净采购成本、回本比例和已确认亏损。
- 亏损由账本自动计算：`max(0, 周期采购费 - 封禁前收入 - 已到账退款)`；不新增可与周期金额互相矛盾的手填亏损字段。
- 全局利润在封禁时点一次性确认剩余未摊销采购成本，封禁后不再继续日摊销；后续退款在实际到账日作为负成本冲减。
- 提供受确认保护的结算撤销能力用于纠正误标，保留撤销时间和原因；撤销不自动重新开启账号调度。
- 封禁、退款、退款冲正或结算撤销成功后立即失效利润快照，下一次查询重算。

## Impact

- Affected specs: `account-profit-accounting`
- Affected code: `backend/migrations/`、`backend/internal/repository/profit_repo.go`、`backend/internal/service/profit_port.go`、`backend/internal/service/profit_service.go`、`backend/internal/handler/admin/profit_handler.go`、`backend/internal/server/routes/admin.go`、`frontend/src/api/admin/profit.ts`、`frontend/src/components/admin/account/AccountCostConfigDialog.vue`、`frontend/src/components/admin/account/AccountStatsModal.vue`、中英文国际化与相关测试
- **Behavioral change**: 封禁结算会立即终止账号供给，并将剩余未摊销成本提前确认在封禁时点；未结算账号的现有计算不变。
