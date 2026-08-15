#!/usr/bin/env python3
"""Deterministic HTTP/SSE upstream for isolated dual-version reconciliation."""

from __future__ import annotations

import json
import os
import ssl
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any
from urllib.parse import urlparse


def compact(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002
        return

    def send_json(self, value: Any, status: int = 200) -> None:
        body = compact(value)
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def send_sse(self, events: list[dict[str, Any]], done: bool = False) -> None:
        lines: list[bytes] = []
        for event in events:
            event_type = event.get("type")
            if isinstance(event_type, str):
                lines.append(f"event: {event_type}\n".encode())
            lines.append(b"data: " + compact(event) + b"\n\n")
        if done:
            lines.append(b"data: [DONE]\n\n")
        body = b"".join(lines)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        if self.path == "/health":
            self.send_json({"status": "ok"})
            return
        self.send_json({"error": {"message": "not found", "type": "not_found"}}, 404)

    def do_POST(self) -> None:  # noqa: N802
        length = int(self.headers.get("Content-Length", "0"))
        try:
            request = json.loads(self.rfile.read(length) or b"{}")
        except json.JSONDecodeError:
            self.send_json({"error": {"message": "invalid json", "type": "invalid_request_error"}}, 400)
            return

        path = urlparse(self.path).path
        if path.endswith("/messages"):
            self.handle_anthropic(request)
            return
        if path.endswith("/responses"):
            self.handle_responses(request)
            return
        if path.endswith("/chat/completions"):
            self.handle_chat_completions(request)
            return
        self.send_json({"error": {"message": "not found", "type": "not_found"}}, 404)

    def handle_anthropic(self, request: dict[str, Any]) -> None:
        model = str(request.get("model", "unknown"))
        usage = {
            "input_tokens": 10,
            "cache_creation_input_tokens": 2,
            "cache_read_input_tokens": 3,
            "output_tokens": 2 if request.get("stream") else 1,
        }
        message = {
            "id": "msg_deterministic",
            "type": "message",
            "role": "assistant",
            "model": model,
            "content": [{"type": "text", "text": "OK"}],
            "stop_reason": "end_turn",
            "stop_sequence": None,
            "usage": usage,
        }
        if not request.get("stream"):
            self.send_json(message)
            return
        self.send_sse(
            [
                {"type": "message_start", "message": {**message, "content": [], "usage": {**usage, "output_tokens": 0}}},
                {"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "OK"}},
                {"type": "content_block_stop", "index": 0},
                {
                    "type": "message_delta",
                    "delta": {"stop_reason": "end_turn", "stop_sequence": None},
                    "usage": usage,
                },
                {"type": "message_stop"},
            ]
        )

    def handle_responses(self, request: dict[str, Any]) -> None:
        model = str(request.get("model", "unknown"))
        usage = {
            "input_tokens": 20,
            "input_tokens_details": {"cached_tokens": 5},
            "output_tokens": 4,
            "output_tokens_details": {"reasoning_tokens": 1},
            "total_tokens": 24,
        }
        response = {
            "id": "resp_deterministic",
            "object": "response",
            "created_at": 1700000000,
            "status": "completed",
            "model": model,
            "output": [
                {
                    "id": "msg_deterministic",
                    "type": "message",
                    "status": "completed",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "OK", "annotations": []}],
                }
            ],
            "usage": usage,
        }
        if not request.get("stream"):
            self.send_json(response)
            return
        self.send_sse(
            [
                {"type": "response.created", "sequence_number": 0, "response": {**response, "status": "in_progress", "output": [], "usage": None}},
                {"type": "response.in_progress", "sequence_number": 1, "response": {**response, "status": "in_progress", "output": [], "usage": None}},
                {"type": "response.output_item.added", "sequence_number": 2, "output_index": 0, "item": response["output"][0]},
                {"type": "response.output_text.delta", "sequence_number": 3, "item_id": "msg_deterministic", "output_index": 0, "content_index": 0, "delta": "OK"},
                {"type": "response.output_text.done", "sequence_number": 4, "item_id": "msg_deterministic", "output_index": 0, "content_index": 0, "text": "OK"},
                {"type": "response.output_item.done", "sequence_number": 5, "output_index": 0, "item": response["output"][0]},
                {"type": "response.completed", "sequence_number": 6, "response": response},
            ],
            done=True,
        )

    def handle_chat_completions(self, request: dict[str, Any]) -> None:
        model = str(request.get("model", "unknown"))
        usage = {
            "prompt_tokens": 12,
            "prompt_tokens_details": {"cached_tokens": 4},
            "completion_tokens": 3,
            "completion_tokens_details": {"reasoning_tokens": 1},
            "total_tokens": 15,
        }
        response = {
            "id": "chatcmpl_deterministic",
            "object": "chat.completion",
            "created": 1700000000,
            "model": model,
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            "usage": usage,
        }
        if not request.get("stream"):
            self.send_json(response)
            return
        self.send_sse(
            [
                {
                    "id": response["id"],
                    "object": "chat.completion.chunk",
                    "created": response["created"],
                    "model": model,
                    "choices": [{"index": 0, "delta": {"role": "assistant", "content": "OK"}, "finish_reason": None}],
                },
                {
                    "id": response["id"],
                    "object": "chat.completion.chunk",
                    "created": response["created"],
                    "model": model,
                    "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                    "usage": usage,
                },
            ],
            done=True,
        )


if __name__ == "__main__":
    server = ThreadingHTTPServer(("0.0.0.0", 9000), Handler)
    cert_file = os.environ.get("TLS_CERT_FILE", "")
    key_file = os.environ.get("TLS_KEY_FILE", "")
    if cert_file and key_file:
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.load_cert_chain(cert_file, key_file)
        server.socket = context.wrap_socket(server.socket, server_side=True)
    server.serve_forever()
