#!/usr/bin/env python3
"""Drive compare_openai_capacity_ab.py against captured Codex SSE dumps."""

from __future__ import annotations

import io
import json
import subprocess
import sys
import unittest
from contextlib import redirect_stdout
from pathlib import Path

TOOLS = Path(__file__).resolve().parent
FIXTURES = TOOLS / "testdata" / "openai_capacity_ab"
SCRIPT = TOOLS / "compare_openai_capacity_ab.py"

sys.path.insert(0, str(TOOLS))
import compare_openai_capacity_ab as cmp  # noqa: E402


class CompareOpenAICapacityABTest(unittest.TestCase):
    def test_fixtures_are_redacted_and_labeled(self) -> None:
        cap = json.loads((FIXTURES / "capacity.json").read_text())
        other = json.loads((FIXTURES / "ok.json").read_text())
        blob = (FIXTURES / "capacity.json").read_text() + (FIXTURES / "ok.json").read_text()
        self.assertNotIn("Bearer ", blob)
        self.assertNotIn("cookie", blob.lower())
        self.assertNotIn('"instructions"', blob)
        self.assertEqual(cap["label"], "capacity")
        self.assertEqual(cap["error_code"], "server_is_overloaded")
        self.assertIn("at capacity", cap.get("cli_mapped_message", "").lower())
        self.assertEqual(other["label"], "ok")
        self.assertIsNone(other.get("error_code"))
        self.assertEqual(other["event_type"], "response.completed")
        self.assertNotEqual(cap["error_code"], other.get("error_code"))

    def test_freq_counts_values(self) -> None:
        self.assertEqual(cmp.freq(["a", "a", "b"]), "a×2, b×1")

    def test_diff_wire_fields_names_error_code(self) -> None:
        records = cmp.load_records(FIXTURES)
        grouped = cmp.group(records)
        label, contrast = cmp.pick_contrast(grouped)
        self.assertEqual(label, "ok")
        buf = io.StringIO()
        with redirect_stdout(buf):
            diffs = cmp.diff_wire_fields(grouped["capacity"], contrast, label)
        text = buf.getvalue()
        self.assertTrue(any(item.startswith("error_code:") for item in diffs), diffs)
        self.assertIn("server_is_overloaded", text)
        self.assertIn("response.completed", text)
        self.assertIn("DIFF", text)

    def test_script_main_prints_named_difference(self) -> None:
        proc = subprocess.run(
            [sys.executable, str(SCRIPT), str(FIXTURES)],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("named_differences:", proc.stdout)
        self.assertIn("error_code: capacity=server_is_overloaded", proc.stdout)
        self.assertIn("ok=<empty>", proc.stdout)
        self.assertIn("response.completed", proc.stdout)
        self.assertNotIn("Bearer ", proc.stdout)


if __name__ == "__main__":
    unittest.main()
