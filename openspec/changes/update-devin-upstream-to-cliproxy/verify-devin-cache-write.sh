#!/usr/bin/env bash
# 验证 Devin 侧车把「首次出现的提示词」按真实输入计费，而不是记成 3 个 token。
# 在生产机执行：需要 docker 访问 sub2api-postgres 与 sub2api-devin-cliproxy。
# 用全新随机内容发起请求（首次出现 → cache write 路径），断言 input_tokens 不为 3。
set -euo pipefail

KEY=$(docker exec sub2api-postgres psql -U sub2api -d sub2api -tAc \
  "SELECT key FROM api_keys WHERE id=10 AND status='active'")
[ -n "$KEY" ] || { echo "FAIL: no active client key for routing to account 126"; exit 1; }

PROMPT=$(mktemp)
trap 'rm -f "$PROMPT"' EXIT
python3 - "$PROMPT" <<'PY'
import json, sys, time
nonce = str(time.time_ns())
text = f"nonce {nonce}. Reply with the single word ok. " + (
    f"verify cache write {nonce} violet harbor quartz. " * 170)
open(sys.argv[1], "w").write(json.dumps(
    {"model": "gpt-5.6-terra", "input": text, "max_output_tokens": 16, "stream": False}))
PY

curl -sS --resolve code.sicts.shop:443:127.0.0.1 https://code.sicts.shop/v1/responses \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  --data-binary @"$PROMPT" -o /tmp/verify-devin-cache-write.json

python3 - <<'PY'
import json
u = (json.load(open("/tmp/verify-devin-cache-write.json")).get("usage") or {})
inp = u.get("input_tokens") or 0
cached = (u.get("input_tokens_details") or {}).get("cached_tokens")
print("input_tokens", inp, "cached_tokens", cached)
if inp < 500:
    raise SystemExit("FAIL: first-seen prompt billed as %d input tokens" % inp)
print("PASS: first-seen prompt billed as real input tokens")
PY
