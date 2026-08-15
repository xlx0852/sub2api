#!/usr/bin/env python3
"""Deterministic six-provider upstream for settlement durability tests.

The server is test-only. It stores no credentials and exposes a small local
control surface so a crash harness can switch between complete and incomplete
terminal responses without changing production configuration keys.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import select
import socket
import socketserver
import ssl
import struct
import threading
import zlib
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any
from urllib.parse import urlparse


SCENARIOS = {
    "normal",
    "missing_terminal",
    "retrieve_404",
    "hold_before_terminal",
    "hold_after_terminal",
    "http_401",
    "http_403",
    "http_429",
    "http_500",
}


def compact(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()


@dataclass
class FixtureState:
    scenario: str = "normal"
    requests: list[dict[str, Any]] = field(default_factory=list)
    stored_responses: dict[str, dict[str, Any]] = field(default_factory=dict)
    connection_providers: dict[tuple[str, int], str] = field(default_factory=dict)
    lock: threading.Lock = field(default_factory=threading.Lock)
    barrier: threading.Condition = field(init=False)
    terminal_delivery_count: int = 0
    partial_delivery_count: int = 0
    terminal_released: bool = False
    barrier_generation: int = 0

    def __post_init__(self) -> None:
        self.barrier = threading.Condition(self.lock)

    def snapshot(self) -> dict[str, Any]:
        with self.lock:
            request_paths: dict[str, int] = {}
            for request in self.requests:
                key = f"{request['method']} {request['path']}"
                request_paths[key] = request_paths.get(key, 0) + 1
            return {
                "scenario": self.scenario,
                "request_count": len(self.requests),
                "request_paths": request_paths,
                "stored_response_ids": sorted(self.stored_responses),
                "terminal_delivery_count": self.terminal_delivery_count,
                "partial_delivery_count": self.partial_delivery_count,
                "terminal_released": self.terminal_released,
            }

    def reset(self, scenario: str = "normal") -> None:
        if scenario not in SCENARIOS:
            raise ValueError(f"unsupported scenario: {scenario}")
        with self.barrier:
            self.barrier_generation += 1
            self.scenario = scenario
            self.requests.clear()
            self.stored_responses.clear()
            self.terminal_delivery_count = 0
            self.partial_delivery_count = 0
            self.terminal_released = False
            self.barrier.notify_all()

    def reach_terminal_barrier(self) -> None:
        with self.barrier:
            if self.scenario not in {"hold_before_terminal", "hold_after_terminal"}:
                return
            generation = self.barrier_generation
            self.terminal_delivery_count += 1
            self.barrier.notify_all()
            while self.scenario == "hold_after_terminal" and generation == self.barrier_generation and not self.terminal_released:
                self.barrier.wait(timeout=1)

    def reach_before_terminal_barrier(self) -> None:
        with self.barrier:
            if self.scenario != "hold_before_terminal":
                return
            generation = self.barrier_generation
            self.partial_delivery_count += 1
            self.barrier.notify_all()
            while generation == self.barrier_generation and not self.terminal_released:
                self.barrier.wait(timeout=1)

    def release_terminal_barrier(self) -> None:
        with self.barrier:
            self.terminal_released = True
            self.barrier.notify_all()

    def record(self, method: str, path: str, request: dict[str, Any]) -> None:
        with self.lock:
            self.requests.append({"method": method, "path": path, "request": request})

    def store_response(self, response: dict[str, Any]) -> None:
        response_id = str(response.get("id", "")).strip()
        if not response_id:
            return
        with self.lock:
            self.stored_responses[response_id] = response

    def retrieve(self, response_id: str) -> dict[str, Any] | None:
        with self.lock:
            if self.scenario == "retrieve_404":
                return None
            return self.stored_responses.get(response_id)

    def register_connection_provider(self, address: tuple[str, int], provider: str) -> None:
        if not provider:
            return
        with self.lock:
            self.connection_providers[address] = provider

    def unregister_connection_provider(self, address: tuple[str, int]) -> None:
        with self.lock:
            self.connection_providers.pop(address, None)

    def provider_for_connection(self, address: tuple[str, int]) -> str:
        with self.lock:
            return self.connection_providers.get(address, "")


def provider_from_path(path: str) -> str:
    if "/model/" in path and (path.endswith("/invoke") or path.endswith("/invoke-with-response-stream")):
        return "bedrock"
    for provider in ("anthropic", "openai", "gemini", "antigravity", "grok", "kimi"):
        if f"/{provider}/" in path or path.startswith(f"/{provider}"):
            return provider
    if "v1internal:" in path:
        return "antigravity"
    if "generateContent" in path:
        return "gemini"
    if path.endswith("/messages"):
        return "anthropic"
    if path.endswith("/chat/completions"):
        return "kimi"
    return "openai"


def provider_from_host(host: str) -> str:
    normalized = host.strip().lower().rstrip(".")
    if normalized.count(":") == 1:
        normalized = normalized.rsplit(":", 1)[0]
    if normalized == "api.x.ai" or normalized.endswith(".x.ai"):
        return "grok"
    return ""


def remember_fixture_server_name(tls_socket: ssl.SSLSocket, server_name: str | None, _context: ssl.SSLContext) -> None:
    setattr(tls_socket, "fixture_provider", provider_from_host(server_name or ""))


def usage_for(provider: str) -> dict[str, Any]:
    if provider in {"anthropic", "bedrock"}:
        return {
            "input_tokens": 10,
            "cache_creation_input_tokens": 2,
            "cache_read_input_tokens": 3,
            "output_tokens": 4,
        }
    if provider in {"gemini", "antigravity"}:
        return {
            "promptTokenCount": 20,
            "cachedContentTokenCount": 5,
            "candidatesTokenCount": 4,
            "thoughtsTokenCount": 1,
            "totalTokenCount": 25,
        }
    if provider == "kimi":
        return {
            "prompt_tokens": 12,
            "prompt_tokens_details": {"cached_tokens": 4},
            "completion_tokens": 3,
            "completion_tokens_details": {"reasoning_tokens": 1},
            "total_tokens": 15,
        }
    return {
        "input_tokens": 20,
        "input_tokens_details": {"cached_tokens": 5},
        "output_tokens": 4,
        "output_tokens_details": {"reasoning_tokens": 1},
        "total_tokens": 24,
    }


class FixtureHTTPServer(ThreadingHTTPServer):
    def __init__(self, server_address: tuple[str, int], state: FixtureState | None = None):
        super().__init__(server_address, FixtureHandler)
        self.fixture_state = state or FixtureState()


class FixedConnectProxy(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True

    def __init__(self, server_address: tuple[str, int], target: tuple[str, int], state: FixtureState):
        self.target = target
        self.fixture_state = state
        super().__init__(server_address, FixedConnectProxyHandler)


class FixedConnectProxyHandler(socketserver.BaseRequestHandler):
    server: FixedConnectProxy

    def handle(self) -> None:
        self.request.settimeout(5)
        header = b""
        while b"\r\n\r\n" not in header and len(header) < 16384:
            chunk = self.request.recv(4096)
            if not chunk:
                return
            header += chunk
        request_line = header.split(b"\r\n", 1)[0]
        if not request_line.startswith(b"CONNECT "):
            self.request.sendall(b"HTTP/1.1 405 Method Not Allowed\r\nContent-Length: 0\r\n\r\n")
            return
        parts = request_line.decode("ascii", errors="replace").split()
        authority = parts[1] if len(parts) >= 2 else ""
        provider = provider_from_host(authority)
        with socket.create_connection(self.server.target, timeout=5) as upstream:
            upstream_address = upstream.getsockname()
            self.server.fixture_state.register_connection_provider(upstream_address, provider)
            try:
                self.request.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
                sockets = (self.request, upstream)
                while True:
                    readable, _, _ = select.select(sockets, (), (), 5)
                    if not readable:
                        continue
                    for source in readable:
                        data = source.recv(65536)
                        if not data:
                            return
                        destination = upstream if source is self.request else self.request
                        destination.sendall(data)
            finally:
                self.server.fixture_state.unregister_connection_provider(upstream_address)


class FixtureHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server: FixtureHTTPServer

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002
        return

    @property
    def state(self) -> FixtureState:
        return self.server.fixture_state

    def read_json(self) -> dict[str, Any]:
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length) if length else b"{}"
        value = json.loads(raw)
        if not isinstance(value, dict):
            raise ValueError("request must be a JSON object")
        return value

    def send_json(self, value: Any, status: int = 200, headers: dict[str, str] | None = None) -> None:
        body = compact(value)
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        for key, val in (headers or {}).items():
            self.send_header(key, val)
        self.end_headers()
        self.wfile.write(body)

    def encode_sse(self, events: list[dict[str, Any]], done: bool = False) -> bytes:
        chunks: list[bytes] = []
        for event in events:
            event_type = event.get("type")
            if isinstance(event_type, str):
                chunks.append(f"event: {event_type}\n".encode())
            chunks.append(b"data: " + compact(event) + b"\n\n")
        if done:
            chunks.append(b"data: [DONE]\n\n")
        return b"".join(chunks)

    def write_chunk(self, body: bytes) -> None:
        if body:
            self.wfile.write(f"{len(body):X}\r\n".encode() + body + b"\r\n")
            self.wfile.flush()

    def send_sse(self, events: list[dict[str, Any]], done: bool = False, terminal_event_count: int = 1) -> None:
        if self.state.scenario == "hold_before_terminal" and len(events) > terminal_event_count:
            split = len(events) - terminal_event_count
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Transfer-Encoding", "chunked")
            self.send_header("x-request-id", "req_fixture_terminal")
            self.send_header("x-goog-request-id", "goog_fixture_terminal")
            self.send_header("xai-request-id", "xai_fixture_terminal")
            self.end_headers()
            self.write_chunk(self.encode_sse(events[:split]))
            self.state.reach_before_terminal_barrier()
            self.write_chunk(self.encode_sse(events[split:], done=done))
            self.wfile.write(b"0\r\n\r\n")
            self.wfile.flush()
            return
        body = self.encode_sse(events, done=done)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("x-request-id", "req_fixture_terminal")
        self.send_header("x-goog-request-id", "goog_fixture_terminal")
        self.send_header("xai-request-id", "xai_fixture_terminal")
        self.end_headers()
        self.wfile.write(body)

    def encode_eventstream_header(self, name: str, value: str) -> bytes:
        name_bytes = name.encode()
        value_bytes = value.encode()
        return bytes([len(name_bytes)]) + name_bytes + b"\x07" + struct.pack("!H", len(value_bytes)) + value_bytes

    def encode_bedrock_eventstream_frame(self, event: dict[str, Any]) -> bytes:
        headers = b"".join([
            self.encode_eventstream_header(":message-type", "event"),
            self.encode_eventstream_header(":event-type", "chunk"),
            self.encode_eventstream_header(":content-type", "application/json"),
        ])
        payload = compact({"bytes": base64.b64encode(compact(event)).decode()})
        total_length = 12 + len(headers) + len(payload) + 4
        prelude_fields = struct.pack("!II", total_length, len(headers))
        prelude = prelude_fields + struct.pack("!I", zlib.crc32(prelude_fields) & 0xFFFFFFFF)
        message = prelude + headers + payload
        return message + struct.pack("!I", zlib.crc32(message) & 0xFFFFFFFF)

    def send_bedrock_eventstream(self, events: list[dict[str, Any]], terminal_event_count: int = 2) -> None:
        self.send_response(200)
        self.send_header("Content-Type", "application/vnd.amazon.eventstream")
        self.send_header("Transfer-Encoding", "chunked")
        self.send_header("x-amzn-requestid", "amzn_fixture_bedrock")
        self.end_headers()
        split = len(events) - terminal_event_count if self.state.scenario == "hold_before_terminal" else len(events)
        initial = b"".join(self.encode_bedrock_eventstream_frame(event) for event in events[:split])
        terminal = b"".join(self.encode_bedrock_eventstream_frame(event) for event in events[split:])
        self.write_chunk(initial)
        if self.state.scenario == "hold_before_terminal":
            self.state.reach_before_terminal_barrier()
        self.write_chunk(terminal)
        self.wfile.write(b"0\r\n\r\n")
        self.wfile.flush()

    def read_websocket_frame(self) -> tuple[int, bytes] | None:
        first = self.rfile.read(2)
        if len(first) != 2:
            return None
        first_byte, second_byte = first
        opcode = first_byte & 0x0F
        masked = bool(second_byte & 0x80)
        length = second_byte & 0x7F
        if length == 126:
            raw = self.rfile.read(2)
            if len(raw) != 2:
                return None
            length = struct.unpack("!H", raw)[0]
        elif length == 127:
            raw = self.rfile.read(8)
            if len(raw) != 8:
                return None
            length = struct.unpack("!Q", raw)[0]
        mask = self.rfile.read(4) if masked else b""
        payload = self.rfile.read(length)
        if len(payload) != length:
            return None
        if masked:
            payload = bytes(value ^ mask[index % 4] for index, value in enumerate(payload))
        return opcode, payload

    def send_websocket_frame(self, opcode: int, payload: bytes = b"") -> None:
        header = bytearray([0x80 | (opcode & 0x0F)])
        length = len(payload)
        if length < 126:
            header.append(length)
        elif length <= 0xFFFF:
            header.append(126)
            header.extend(struct.pack("!H", length))
        else:
            header.append(127)
            header.extend(struct.pack("!Q", length))
        self.wfile.write(bytes(header) + payload)
        self.wfile.flush()

    def send_websocket_json(self, value: dict[str, Any]) -> None:
        self.send_websocket_frame(1, compact(value))

    def handle_websocket(self, path: str) -> None:
        key = self.headers.get("Sec-WebSocket-Key", "").strip()
        if not key:
            self.send_error(400, "missing Sec-WebSocket-Key")
            return
        accept = base64.b64encode(hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest()).decode()
        self.send_response(101, "Switching Protocols")
        self.send_header("Upgrade", "websocket")
        self.send_header("Connection", "Upgrade")
        self.send_header("Sec-WebSocket-Accept", accept)
        self.end_headers()
        self.wfile.flush()
        self.close_connection = True
        turn = 0
        while True:
            frame = self.read_websocket_frame()
            if frame is None:
                return
            opcode, payload = frame
            if opcode == 8:
                self.send_websocket_frame(8, payload[:125])
                return
            if opcode == 9:
                self.send_websocket_frame(10, payload[:125])
                continue
            if opcode != 1:
                continue
            try:
                request = json.loads(payload)
            except json.JSONDecodeError:
                self.send_websocket_json({"type": "error", "error": {"type": "invalid_request_error", "message": "invalid JSON"}})
                continue
            if not isinstance(request, dict):
                continue
            event_type = str(request.get("type", "response.create"))
            if event_type != "response.create":
                self.send_websocket_json({"type": "error", "error": {"type": "invalid_request_error", "message": f"unsupported event: {event_type}"}})
                continue
            turn += 1
            self.state.record("WS", path, request)
            model = str(request.get("model", "gpt-fixture"))
            response_id = f"resp_fixture_ws_{turn}"
            usage = usage_for("openai")
            response = {
                "id": response_id,
                "object": "response",
                "status": "completed",
                "model": model,
                "output": [{
                    "id": f"msg_fixture_ws_{turn}",
                    "type": "message",
                    "status": "completed",
                    "role": "assistant",
                    "content": [{"type": "output_text", "text": "OK", "annotations": []}],
                }],
                "usage": usage,
            }
            self.state.store_response(response)
            self.send_websocket_json({
                "type": "response.created",
                "sequence_number": 0,
                "response": {**response, "status": "in_progress", "output": [], "usage": None},
            })
            self.send_websocket_json({
                "type": "response.output_text.delta",
                "sequence_number": 1,
                "item_id": f"msg_fixture_ws_{turn}",
                "output_index": 0,
                "content_index": 0,
                "delta": "OK",
            })
            if self.state.scenario == "missing_terminal":
                continue
            self.send_websocket_json({"type": "response.completed", "sequence_number": 2, "response": response})
            self.state.reach_terminal_barrier()

    def do_GET(self) -> None:  # noqa: N802
        path = urlparse(self.path).path
        if self.headers.get("Upgrade", "").lower() == "websocket":
            self.handle_websocket(path)
            return
        if path == "/health":
            self.send_json({"status": "ok", **self.state.snapshot()})
            return
        if path == "/__control":
            self.send_json(self.state.snapshot())
            return
        if path == "/__requests":
            with self.state.lock:
                requests = list(self.state.requests)
            self.send_json({"requests": requests})
            return
        marker = "/v1/responses/"
        if marker in path:
            response_id = path.rsplit(marker, 1)[1]
            response = self.state.retrieve(response_id)
            if response is None:
                self.send_json({"error": {"type": "not_found", "message": "response not found"}}, 404)
                return
            self.send_json(response, headers={"x-request-id": "req_fixture_retrieve"})
            return
        self.send_json({"error": {"type": "not_found", "message": "not found"}}, 404)

    def do_POST(self) -> None:  # noqa: N802
        path = urlparse(self.path).path
        try:
            request = self.read_json()
        except (json.JSONDecodeError, ValueError) as error:
            self.send_json({"error": {"type": "invalid_request_error", "message": str(error)}}, 400)
            return
        if path == "/__control":
            action = str(request.get("action", "reset"))
            if action == "release_terminal":
                self.state.release_terminal_barrier()
                self.send_json(self.state.snapshot())
                return
            scenario = str(request.get("scenario", "normal"))
            try:
                self.state.reset(scenario)
            except ValueError as error:
                self.send_json({"error": {"type": "invalid_scenario", "message": str(error)}}, 400)
                return
            self.send_json(self.state.snapshot())
            return

        provider = str(self.headers.get("X-Sub2API-Fixture-Provider", "")).strip().lower()
        if provider not in {"anthropic", "openai", "gemini", "antigravity", "grok", "kimi", "bedrock"}:
            provider = (
                self.state.provider_for_connection(self.client_address)
                or
                str(getattr(self.connection, "fixture_provider", ""))
                or provider_from_host(str(self.headers.get("Host", "")))
                or provider_from_path(path)
            )
        self.state.record("POST", path, request)

        fault_status = {
            "http_401": 401,
            "http_403": 403,
            "http_429": 429,
            "http_500": 500,
        }.get(self.state.scenario)
        if fault_status is not None and "/fault/" in path:
            headers = {"retry-after": "1"} if fault_status == 429 else None
            self.send_json(
                {"error": {"type": f"fixture_http_{fault_status}", "message": "injected scheduler failover fault"}},
                fault_status,
                headers=headers,
            )
            return

        if provider == "bedrock":
            self.handle_bedrock(request)
            return
        if provider == "anthropic" or path.endswith("/messages"):
            self.handle_anthropic(request)
            return
        if provider in {"gemini", "antigravity"} or "generateContent" in path or "v1internal:" in path:
            self.handle_gemini(provider, request)
            return
        if provider == "kimi" or path.endswith("/chat/completions"):
            self.handle_chat(request)
            return
        self.handle_responses(provider, request)

    def handle_anthropic(self, request: dict[str, Any]) -> None:
        usage = usage_for("anthropic")
        model = str(request.get("model", "claude-fixture"))
        message = {
            "id": "msg_fixture_anthropic",
            "type": "message",
            "role": "assistant",
            "model": model,
            "content": [{"type": "text", "text": "OK"}],
            "stop_reason": "end_turn",
            "stop_sequence": None,
            "usage": usage,
        }
        headers = {"x-request-id": "req_fixture_anthropic"}
        if not request.get("stream"):
            if self.state.scenario == "missing_terminal":
                self.send_json({**message, "usage": None, "stop_reason": None}, headers=headers)
                return
            self.send_json(message, headers=headers)
            self.state.reach_terminal_barrier()
            return
        start = {"type": "message_start", "message": {**message, "content": [], "stop_reason": None, "usage": {**usage, "output_tokens": 0}}}
        if self.state.scenario == "missing_terminal":
            self.send_sse([start, {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "OK"}}])
            return
        self.send_sse([
            start,
            {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "OK"}},
            {"type": "message_delta", "delta": {"stop_reason": "end_turn"}, "usage": usage},
            {"type": "message_stop"},
        ], terminal_event_count=2)
        self.state.reach_terminal_barrier()

    def handle_bedrock(self, request: dict[str, Any]) -> None:
        usage = usage_for("bedrock")
        stream = urlparse(self.path).path.endswith("/invoke-with-response-stream")
        response = {
            "id": "msg_fixture_bedrock",
            "type": "message",
            "role": "assistant",
            "model": "bedrock-fixture-model",
            "content": [{"type": "text", "text": "OK"}],
            "stop_reason": "end_turn",
            "stop_sequence": None,
            "usage": usage,
        }
        if not stream:
            if self.state.scenario == "missing_terminal":
                self.send_json({**response, "usage": None, "stop_reason": None}, headers={"x-amzn-requestid": "amzn_fixture_bedrock"})
                return
            self.send_json(response, headers={"x-amzn-requestid": "amzn_fixture_bedrock"})
            self.state.reach_terminal_barrier()
            return
        start = {
            "type": "message_start",
            "message": {**response, "content": [], "stop_reason": None, "usage": {**usage, "output_tokens": 0}},
        }
        delta = {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": "OK"}}
        if self.state.scenario == "missing_terminal":
            self.send_bedrock_eventstream([start, delta], terminal_event_count=0)
            return
        terminal = {
            "type": "message_delta",
            "delta": {"stop_reason": "end_turn"},
            "amazon-bedrock-invocationMetrics": {"inputTokenCount": 10, "outputTokenCount": 4},
        }
        self.send_bedrock_eventstream([start, delta, terminal, {"type": "message_stop"}], terminal_event_count=2)
        self.state.reach_terminal_barrier()

    def handle_responses(self, provider: str, request: dict[str, Any]) -> None:
        usage = usage_for(provider)
        model = str(request.get("model", f"{provider}-fixture"))
        response_id = f"resp_fixture_{provider}"
        is_capability_probe = request.get("tool_choice") == "required" and any(
            isinstance(tool, dict) and tool.get("name") == "probe_ping"
            for tool in request.get("tools", [])
        )
        output = [{
            "id": f"call_fixture_{provider}",
            "type": "function_call",
            "status": "completed",
            "call_id": f"call_fixture_{provider}",
            "name": "probe_ping",
            "arguments": '{"ok":true}',
        }] if is_capability_probe else [{
            "id": f"msg_fixture_{provider}",
            "type": "message",
            "status": "completed",
            "role": "assistant",
            "content": [{"type": "output_text", "text": "OK", "annotations": []}],
        }]
        response = {
            "id": response_id,
            "object": "response",
            "created_at": 1700000000,
            "status": "completed",
            "model": model,
            "output": output,
            "usage": usage,
            "store": bool(request.get("store", True)),
        }
        if response["store"]:
            self.state.store_response(response)
        headers = {"x-request-id": f"req_fixture_{provider}", "xai-request-id": f"xai_fixture_{provider}"}
        if not request.get("stream"):
            if self.state.scenario == "missing_terminal":
                self.send_json({**response, "status": "in_progress", "usage": None}, headers=headers)
                return
            self.send_json(response, headers=headers)
            self.state.reach_terminal_barrier()
            return
        created = {"type": "response.created", "sequence_number": 0, "response": {**response, "status": "in_progress", "output": [], "usage": None}}
        delta = {"type": "response.output_text.delta", "sequence_number": 1, "item_id": f"msg_fixture_{provider}", "output_index": 0, "content_index": 0, "delta": "OK"}
        if self.state.scenario == "missing_terminal":
            self.send_sse([created, delta])
            return
        self.send_sse([created, delta, {"type": "response.completed", "sequence_number": 2, "response": response}], done=True)
        self.state.reach_terminal_barrier()

    def handle_chat(self, request: dict[str, Any]) -> None:
        usage = usage_for("kimi")
        model = str(request.get("model", "kimi-fixture"))
        response = {
            "id": "chatcmpl_fixture_kimi",
            "object": "chat.completion",
            "created": 1700000000,
            "model": model,
            "choices": [{"index": 0, "message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
            "usage": usage,
        }
        headers = {"x-request-id": "req_fixture_kimi"}
        if not request.get("stream"):
            if self.state.scenario == "missing_terminal":
                self.send_json({**response, "usage": None}, headers=headers)
                return
            self.send_json(response, headers=headers)
            self.state.reach_terminal_barrier()
            return
        first = {"id": response["id"], "object": "chat.completion.chunk", "created": response["created"], "model": model, "choices": [{"index": 0, "delta": {"content": "OK"}, "finish_reason": None}]}
        if self.state.scenario == "missing_terminal":
            self.send_sse([first])
            return
        terminal = {"id": response["id"], "object": "chat.completion.chunk", "created": response["created"], "model": model, "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}], "usage": usage}
        self.send_sse([first, terminal], done=True)
        self.state.reach_terminal_barrier()

    def handle_gemini(self, provider: str, request: dict[str, Any]) -> None:
        usage = usage_for(provider)
        response_id = f"gemini_fixture_{provider}"
        inner = {
            "candidates": [{"content": {"role": "model", "parts": [{"text": "OK"}]}, "finishReason": "STOP", "index": 0}],
            "usageMetadata": usage,
            "responseId": response_id,
            "modelVersion": f"{provider}-fixture-model",
        }
        value = {"response": inner, "responseId": response_id, "modelVersion": inner["modelVersion"]} if provider == "antigravity" else inner
        stream = "streamGenerateContent" in urlparse(self.path).path or "alt=sse" in self.path
        headers = {"x-request-id": f"req_fixture_{provider}", "x-goog-request-id": f"goog_fixture_{provider}"}
        if not stream:
            if self.state.scenario == "missing_terminal":
                broken_inner = {**inner, "usageMetadata": None, "candidates": [{"content": {"role": "model", "parts": [{"text": "OK"}]}}]}
                broken = {"response": broken_inner, "responseId": response_id} if provider == "antigravity" else broken_inner
                self.send_json(broken, headers=headers)
                return
            self.send_json(value, headers=headers)
            self.state.reach_terminal_barrier()
            return
        partial_inner = {**inner, "usageMetadata": None, "candidates": [{"content": {"role": "model", "parts": [{"text": "OK"}]}}]}
        partial = {"response": partial_inner, "responseId": response_id} if provider == "antigravity" else partial_inner
        if self.state.scenario == "missing_terminal":
            self.send_sse([partial])
            return
        self.send_sse([partial, value])
        self.state.reach_terminal_barrier()


def make_server(host: str = "127.0.0.1", port: int = 0, state: FixtureState | None = None) -> FixtureHTTPServer:
    return FixtureHTTPServer((host, port), state)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=59000)
    parser.add_argument("--tls-cert")
    parser.add_argument("--tls-key")
    parser.add_argument("--connect-proxy-port", type=int)
    args = parser.parse_args()
    if bool(args.tls_cert) != bool(args.tls_key):
        parser.error("--tls-cert and --tls-key must be provided together")
    server = make_server(args.host, args.port)
    scheme = "http"
    if args.tls_cert and args.tls_key:
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.load_cert_chain(args.tls_cert, args.tls_key)
        context.set_servername_callback(remember_fixture_server_name)
        server.socket = context.wrap_socket(server.socket, server_side=True)
        scheme = "https"
    proxy = None
    proxy_thread = None
    if args.connect_proxy_port:
        proxy = FixedConnectProxy((args.host, args.connect_proxy_port), (args.host, server.server_port), server.fixture_state)
        proxy_thread = threading.Thread(target=proxy.serve_forever, daemon=True)
        proxy_thread.start()
        print(f"fixed CONNECT proxy listening on http://{args.host}:{args.connect_proxy_port}", flush=True)
    print(f"usage settlement fixture listening on {scheme}://{args.host}:{server.server_port}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        if proxy is not None:
            proxy.shutdown()
            proxy.server_close()
        if proxy_thread is not None:
            proxy_thread.join(timeout=2)
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
