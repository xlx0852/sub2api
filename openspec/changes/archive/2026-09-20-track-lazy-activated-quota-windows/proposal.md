# Change: 记录延迟激活的真实配额窗口

## Why

订阅账号的上游周额度会在厂商清零时结束旧窗口，但新的滚动窗口可能要等到第一次真实模型请求才开始。现有账本会把这段等待时间并入旧窗口或投影成新窗口，无法真实反映厂商窗口和订阅空闲成本。

## What Changes

- 以已记录的付费订阅周期作为可用性和成本的绝对边界。
- 区分“厂商清零”和“新窗口激活”：清零只关闭旧窗口，真实请求观测到新倒计时后才开窗。
- 将两个真实窗口之间的时间表示为 `waiting_activation` 空档，不归属旧窗口或新窗口。
- 改为按上游 `reset_at` 跳变识别提前重置，不再要求新旧理论窗口不重叠。
- Profit API 返回真实窗口和待激活空档，前端分层展示，并与订阅周期求交集。
- 后台巡检不得使用会激活滚动窗口的模型推理探针。

## Impact

- Affected specs: `account-profit-quota-windows`
- Affected code: `backend/internal/service/account_quota_window.go`, quota probe/sweep wiring, Profit quota-window response assembly, `frontend/src/components/admin/profit/QuotaWindowPanel.vue`, related migrations and tests.

