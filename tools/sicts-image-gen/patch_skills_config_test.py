#!/usr/bin/env python3
"""Tests for patch_skills_config path serialization (Windows TOML safety)."""
from __future__ import annotations

from pathlib import Path

from patch_skills_config import (
    ensure_block,
    path_assignment,
    repair_windows_path_lines,
    toml_path_body,
)


def test_windows_path_uses_single_quotes_and_backslashes():
    p = Path(r"C:\Users\73666\.codex\skills\sicts-imagegen\SKILL.md")
    line = path_assignment(p)
    # Customer-validated form
    assert line.startswith("path = '")
    assert line.endswith("'")
    assert r"C:\Users\73666" in line or "C:\\\\Users\\\\73666" in repr(line)
    assert "C:/Users" not in line
    assert '"' not in line  # no double quotes around path


def test_unix_path_single_quoted_forward_slashes():
    p = Path("/home/u/.codex/skills/sicts-imagegen/SKILL.md")
    line = path_assignment(p)
    assert line == "path = '/home/u/.codex/skills/sicts-imagegen/SKILL.md'"


def test_ensure_block_writes_single_quoted_windows_path():
    skill = Path(r"C:\Users\73666\.codex\skills\sicts-imagegen\SKILL.md")
    text = ensure_block("", skill, True, "test")
    assert "path = 'C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md'" in text
    assert "enabled = true" in text
    # Must NOT write double-quoted backslash form
    assert 'path = "C:\\Users' not in text


def test_repair_customer_broken_double_quotes_to_single():
    # Exactly the broken form that crashed Codex
    broken = (
        'model = "gpt-5.6-sol"\n'
        "\n"
        "[[skills.config]]\n"
        'path = "C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md"\n'
        "enabled = true\n"
        "\n"
        "[[skills.config]]\n"
        'path = "C:\\Users\\73666\\.codex\\skills\\.system\\imagegen\\SKILL.md"\n'
        "enabled = false\n"
    )
    fixed, n = repair_windows_path_lines(broken)
    assert n == 2
    assert (
        "path = 'C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md'" in fixed
    )
    assert (
        "path = 'C:\\Users\\73666\\.codex\\skills\\.system\\imagegen\\SKILL.md'"
        in fixed
    )
    # No double-quoted Windows path left
    assert 'path = "C:' not in fixed


def test_repair_already_single_quoted_is_noop():
    good = (
        "[[skills.config]]\n"
        "path = 'C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md'\n"
        "enabled = true\n"
    )
    fixed, n = repair_windows_path_lines(good)
    assert n == 0
    assert fixed == good


def test_repair_double_escaped_backslashes():
    broken = 'path = "C:\\\\Users\\\\73666\\\\.codex\\\\skills\\\\sicts-imagegen\\\\SKILL.md"\n'
    fixed, n = repair_windows_path_lines(broken)
    assert n == 1
    assert fixed.strip() == "path = 'C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md'"


def test_rewrite_existing_block_to_single_quote():
    skill = Path(r"C:\Users\73666\.codex\skills\sicts-imagegen\SKILL.md")
    text = (
        "[[skills.config]]\n"
        'path = "C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md"\n'
        "enabled = false\n"
    )
    text, _ = repair_windows_path_lines(text)
    out = ensure_block(text, skill, True, "test")
    assert "path = 'C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md'" in out
    assert "enabled = true" in out


def test_toml_path_body_escapes_single_quote():
    p = Path(r"C:\Users\O'Brien\.codex\skills\sicts-imagegen\SKILL.md")
    body = toml_path_body(p)
    assert "O''Brien" in body


if __name__ == "__main__":
    test_windows_path_uses_single_quotes_and_backslashes()
    test_unix_path_single_quoted_forward_slashes()
    test_ensure_block_writes_single_quoted_windows_path()
    test_repair_customer_broken_double_quotes_to_single()
    test_repair_already_single_quoted_is_noop()
    test_repair_double_escaped_backslashes()
    test_rewrite_existing_block_to_single_quote()
    test_toml_path_body_escapes_single_quote()
    print("ok")
