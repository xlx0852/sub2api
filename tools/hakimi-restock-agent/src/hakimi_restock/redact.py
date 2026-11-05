from __future__ import annotations

import logging
import re

_PATTERNS = (
    re.compile(
        r"(?i)(access_token|refresh_token|id_token|session_token|admin_api_key|"
        r"customer_token|api_key|private_key|authorization|token)"
        r"(['\"]?\s*[:=]\s*(?:bearer\s+)?)([^\s'\",]+)"
    ),
    re.compile(r"(?i)bearer\s+\S+"),
    re.compile(r"(?i)(x-customer-token|x-api-key)\s*:\s*\S+"),
    re.compile(r"eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}"),
)


def redact_text(value: str) -> str:
    text = value
    text = _PATTERNS[0].sub(r"\1\2***", text)
    text = _PATTERNS[1].sub("Bearer ***", text)
    text = _PATTERNS[2].sub(r"\1: ***", text)
    text = _PATTERNS[3].sub("***jwt***", text)
    return text


class RedactFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        if isinstance(record.msg, str):
            record.msg = redact_text(record.msg)
        if record.args:
            if isinstance(record.args, dict):
                record.args = {
                    key: redact_text(val) if isinstance(val, str) else val
                    for key, val in record.args.items()
                }
            else:
                record.args = tuple(
                    redact_text(arg) if isinstance(arg, str) else arg
                    for arg in record.args
                )
        return True
