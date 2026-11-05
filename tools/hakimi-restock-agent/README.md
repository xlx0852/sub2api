# Hakimi Restock Agent

独立 Python 进程：用 Hakimi 客户 Token 买号 / 售后，用本机 Sub2API admin key 导入号池。

**不要把 Sub2API admin key 配到 Hakimi Hub。** 本程序也不会调用 Hakimi 的 `/hubs` 或 `/orders/{id}/push`。

## 做什么

1. 读本机号池可调度数
2. 低于水位时查 Hakimi 库存、按下单价冻结预算下单
3. 轮询订单，下载 `format=sub2`
4. 调 Sub2API `POST /api/v1/admin/accounts/import/codex-session`，按身份合并（已有号更新凭据，没有才新建）
5. 定时对已知订单 `recover`，有新凭据再导入同一批号

崩溃后续跑会先恢复未完成订单：同一 `Idempotency-Key` + 原请求体，不会换 Key 重下。

## 安装

```bash
cd tools/hakimi-restock-agent
python3 -m venv .venv
source .venv/bin/activate
pip install -e .
cp config.example.yaml config.yaml
```

密钥用环境变量，不要写进 yaml：

```bash
export HAKIMI_TOKEN='客户 Token'
export SUB2API_BASE_URL='https://your-sub2api-host'
export SUB2API_ADMIN_API_KEY='本机 admin key'
# 或：export SUB2API_JWT='管理员 access_token'
```

`config.yaml` 里改水位、每次买多少、预算（分）、分组、冷却。

## 命令

```bash
# 只看号池和 Hakimi 余额，不买
hakimi-restock -c config.yaml status

# 干跑：报价但不下单
hakimi-restock -c config.yaml --dry-run once

# 跑一轮（可能真买）
hakimi-restock -c config.yaml once

# 常驻
hakimi-restock -c config.yaml run
```

systemd 示例：

```ini
[Service]
WorkingDirectory=/opt/hakimi-restock-agent
EnvironmentFile=/opt/hakimi-restock-agent/.env
ExecStart=/opt/hakimi-restock-agent/.venv/bin/hakimi-restock -c /opt/hakimi-restock-agent/config.yaml run
Restart=always
RestartSec=15
```

## 配置要点

- `buy.enabled`：默认 false。没定补号策略前不要打开，打开才会下单
- `recover.enabled`：默认 false。打开才会对已知订单做售后复活
- `pool.min_schedulable`：可调度低于这个数才买
- `pool.group_id`：只统计这个分组；`0` 表示该平台全部 OAuth
- `buy.budget_cents`：单次最高冻结金额，单位分。报价超预算会拒绝下单
- `import.group_ids`：导入到哪些分组。建议填，并保持 `skip_default_group_bind: true`
- `runtime.save_downloads`：默认 false。打开会把下载文件落到 `data/downloads/`，里面是凭据，注意权限

## 不会做的事

- 不把 admin key 发给 Hakimi
- 不注册 / 检测 Hub
- 不调用模型测试来宣称账号可用
- 不把 Token、AT/RT 打进日志
- 不在网关主进程里跑

## 状态文件

`data/state.json` 记录 inflight 订单、幂等键、已知 order id。可以提交到 git 的只有 example；`config.yaml`、`.env`、`data/` 已 gitignore。
