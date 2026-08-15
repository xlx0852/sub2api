from __future__ import annotations

import json
import pathlib
import sys
import threading
import time
import unittest
import urllib.request
import struct
import zlib
from unittest import mock


TOOLS = pathlib.Path(__file__).resolve().parent
if str(TOOLS) not in sys.path:
    sys.path.insert(0, str(TOOLS))

import usage_settlement_crash_harness as harness  # noqa: E402
from usage_settlement_fake_upstream import (  # noqa: E402
    FixtureState,
    make_server,
    provider_from_host,
    provider_from_path,
    remember_fixture_server_name,
)
from websockets.sync.client import connect  # noqa: E402


class FixtureStateTest(unittest.TestCase):
    def test_reset_rejects_unknown_scenario(self) -> None:
        state = FixtureState()
        with self.assertRaises(ValueError):
            state.reset("unknown")

    def test_stored_response_retrieve_and_forced_miss(self) -> None:
        state = FixtureState()
        state.store_response({"id": "resp_1", "usage": {"input_tokens": 1}})
        self.assertEqual("resp_1", state.retrieve("resp_1")["id"])
        state.scenario = "retrieve_404"
        self.assertIsNone(state.retrieve("resp_1"))

    def test_connection_provider_registration_is_scoped_by_peer(self) -> None:
        state = FixtureState()
        peer = ("127.0.0.1", 43123)
        state.register_connection_provider(peer, "grok")
        self.assertEqual("grok", state.provider_for_connection(peer))
        self.assertEqual("", state.provider_for_connection(("127.0.0.1", 43124)))
        state.unregister_connection_provider(peer)
        self.assertEqual("", state.provider_for_connection(peer))

    def test_provider_path_detection(self) -> None:
        self.assertEqual("antigravity", provider_from_path("/antigravity/v1internal:generateContent"))
        self.assertEqual("gemini", provider_from_path("/gemini/v1beta/models/x:generateContent"))
        self.assertEqual("kimi", provider_from_path("/kimi/v1/chat/completions"))
        self.assertEqual("bedrock", provider_from_path("/model/us.anthropic.claude/invoke-with-response-stream"))

    def test_provider_host_detection_distinguishes_xai_responses(self) -> None:
        self.assertEqual("grok", provider_from_host("api.x.ai"))
        self.assertEqual("grok", provider_from_host("API.X.AI:443"))
        self.assertEqual("grok", provider_from_host("fixture.api.x.ai."))
        self.assertEqual("", provider_from_host("api.openai.com"))

    def test_tls_server_name_is_attached_to_the_fixture_connection(self) -> None:
        connection = mock.Mock()
        remember_fixture_server_name(connection, "api.x.ai", mock.Mock())
        self.assertEqual("grok", connection.fixture_provider)

    def test_terminal_barrier_waits_for_explicit_release(self) -> None:
        state = FixtureState()
        state.reset("hold_after_terminal")
        worker = threading.Thread(target=state.reach_terminal_barrier)
        worker.start()
        deadline = time.monotonic() + 1
        while state.snapshot()["terminal_delivery_count"] == 0 and time.monotonic() < deadline:
            time.sleep(0.01)
        self.assertEqual(1, state.snapshot()["terminal_delivery_count"])
        self.assertTrue(worker.is_alive())
        state.release_terminal_barrier()
        worker.join(timeout=1)
        self.assertFalse(worker.is_alive())

    def test_before_terminal_barrier_waits_after_partial_delivery(self) -> None:
        state = FixtureState()
        state.reset("hold_before_terminal")
        worker = threading.Thread(target=state.reach_before_terminal_barrier)
        worker.start()
        deadline = time.monotonic() + 1
        while state.snapshot()["partial_delivery_count"] == 0 and time.monotonic() < deadline:
            time.sleep(0.01)
        self.assertEqual(1, state.snapshot()["partial_delivery_count"])
        self.assertEqual(0, state.snapshot()["terminal_delivery_count"])
        self.assertTrue(worker.is_alive())
        state.release_terminal_barrier()
        worker.join(timeout=1)
        self.assertFalse(worker.is_alive())


class FixtureSmokeTest(unittest.TestCase):
    def test_six_provider_fixture_smoke(self) -> None:
        self.assertEqual(0, harness.fixture_smoke())

    def test_openai_websocket_two_turns(self) -> None:
        server = make_server()
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            with connect(f"ws://127.0.0.1:{server.server_port}/openai/v1/responses") as socket:
                for turn in (1, 2):
                    socket.send('{"type":"response.create","model":"gpt-fixture","input":"hi"}')
                    events = []
                    while True:
                        event = json.loads(socket.recv())
                        events.append(event)
                        if event.get("type") == "response.completed":
                            break
                    self.assertEqual(f"resp_fixture_ws_{turn}", events[-1]["response"]["id"])
                    self.assertEqual(20, events[-1]["response"]["usage"]["input_tokens"])
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=2)

    def test_openai_responses_capability_probe_returns_function_call(self) -> None:
        server = make_server()
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            status, payload, _ = harness.request_json(
                f"http://127.0.0.1:{server.server_port}/openai/v1/responses",
                "POST",
                {
                    "model": "gpt-fixture",
                    "input": "probe",
                    "tools": [{"type": "function", "name": "probe_ping"}],
                    "tool_choice": "required",
                },
            )
            self.assertEqual(200, status)
            self.assertEqual("function_call", payload["output"][0]["type"])
            self.assertEqual("probe_ping", payload["output"][0]["name"])
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=2)

    def test_bedrock_eventstream_frames_have_valid_crc(self) -> None:
        server = make_server()
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            request = urllib.request.Request(
                f"http://127.0.0.1:{server.server_port}/model/us.anthropic.claude/invoke-with-response-stream",
                data=b"{}",
                method="POST",
                headers={"Content-Type": "application/json"},
            )
            with urllib.request.urlopen(request, timeout=2) as response:
                raw = response.read()
                self.assertEqual("application/vnd.amazon.eventstream", response.headers.get_content_type())
            total_length, headers_length, prelude_crc = struct.unpack("!III", raw[:12])
            self.assertGreater(headers_length, 0)
            self.assertEqual(zlib.crc32(raw[:8]) & 0xFFFFFFFF, prelude_crc)
            first_frame = raw[:total_length]
            self.assertEqual(zlib.crc32(first_frame[:-4]) & 0xFFFFFFFF, struct.unpack("!I", first_frame[-4:])[0])
        finally:
            server.shutdown()
            server.server_close()
            worker.join(timeout=2)


class SettlementSnapshotTest(unittest.TestCase):
    def test_normalized_disconnect_snapshot_removes_only_marker_identity(self) -> None:
        source = {
            "marker_prefix": "run/old",
            "usage_logs": [{"marker": "run/old", "input_tokens": 10}],
            "users": [{"balance": "1.0"}],
        }
        self.assertEqual(
            {
                "usage_logs": [{"input_tokens": 10}],
                "users": [{"balance": "1.0"}],
            },
            harness.normalized_disconnect_snapshot(source),
        )
        self.assertIn("marker_prefix", source)

    def test_sql_literal_escapes_quotes_and_rejects_nul(self) -> None:
        self.assertEqual(harness.sql_literal("run/o'hare"), "'run/o''hare'")
        with self.assertRaises(ValueError):
            harness.sql_literal("bad\x00marker")

    def test_parse_snapshot_output_requires_one_json_object(self) -> None:
        self.assertEqual(harness.parse_snapshot_output(' {"usage_logs": []} \n'), {"usage_logs": []})
        with self.assertRaises(ValueError):
            harness.parse_snapshot_output("{}\n{}\n")
        with self.assertRaises(ValueError):
            harness.parse_snapshot_output("[]\n")

    @mock.patch.object(harness.shutil, "which", return_value="/usr/bin/psql")
    @mock.patch.object(harness.subprocess, "run")
    def test_capture_snapshot_invokes_psql_without_shell(self, run: mock.Mock, _which: mock.Mock) -> None:
        run.return_value = mock.Mock(returncode=0, stdout='{"marker_prefix":"fixture","usage_logs":[]}\n', stderr="")
        dsn = "postgres://fixture@127.0.0.1:55441/sub2api_old"
        self.assertEqual(harness.capture_snapshot(dsn, "fixture", None), 0)
        command = run.call_args.args[0]
        self.assertIn(dsn, command)
        self.assertNotIn("fixture", command[: command.index("--command")])

    @mock.patch.object(harness.subprocess, "run")
    def test_capture_snapshot_rejects_non_loopback_and_empty_marker(self, run: mock.Mock) -> None:
        self.assertEqual(1, harness.capture_snapshot("postgres://production.example/sub2api", "fixture", None))
        self.assertEqual(1, harness.capture_snapshot("postgres://fixture@127.0.0.1/sub2api", "", None))
        run.assert_not_called()

    @mock.patch.object(harness, "request_json")
    def test_http_fixture_control_stays_on_plain_local_fixture(self, request_json: mock.Mock) -> None:
        request_json.return_value = (200, {"scenario": "normal"}, "application/json")
        self.assertEqual(
            (200, {"scenario": "normal"}, "application/json"),
            harness.request_fixture_control("openai", "POST", {"scenario": "normal"}),
        )
        request_json.assert_called_once_with(
            harness.INFRA["fixture"] + "/__control",
            "POST",
            {"scenario": "normal"},
        )


if __name__ == "__main__":
    unittest.main()
