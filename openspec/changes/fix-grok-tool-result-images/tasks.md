## 1. Fix

- [x] 1.1 `grokToolOutputSplit` separates a tool result into text (kept in `function_call_output`) and image parts
  (previously dropped with an explicit "Drop image payloads" comment).
- [x] 1.2 `convertGrokFunctionCallOutputItem(WithCallID)` now returns the tool item plus, when images exist, a user
  message carrying them; the three call sites (`function_call_output`, `custom_tool_call_output`,
  `local_shell_call_output`) append both.
- [x] 1.3 Image parts without a usable URL keep the `[image]` marker; nested `image_url.url` objects are unwrapped.

## 2. Verification

- [x] 2.1 Unit tests `grok_tool_image_test.go`: images forwarded as a user message with the data URL verbatim,
  text-only tool results unchanged, URL-less images keep the marker, nested URL objects unwrapped.
- [x] 2.2 Live reproduction with the operator's own screenshot (915x539, 215 KB, 17-row payment table):
  - before the fix, the tool-result shape answered `5` / `nathan.k@example.net` (fabricated);
  - after the fix, the same request answered `17` / `1468976009@qq.com` (correct);
  - regressions checked on the same image: plain user-message Responses call and `/v1/chat/completions` call both
    still read the image (a 1600x1200 gradient image was also read correctly).
- [x] 2.3 `go build ./...` clean; `internal/service` still shows only the six pre-existing failures.
- [x] 2.4 `openspec validate fix-grok-tool-result-images --strict`.

## Deployment record (2026-09-23)

- Backend `0.1.391-grokimg-3bbabe29f` hot-swapped into the live container's release dir; rollback artifact
  `sub2api.bak-grokimg-*`.
- Temporary admin-owned gateway key used for the live tests was deleted afterwards (0 rows left).
- Known remaining gap, not fixed here: an `input_image` that carries only `file_id` (provider file upload) cannot
  be fetched by this gateway, so the image is absent; clients should send data URLs or `http(s)` URLs.
