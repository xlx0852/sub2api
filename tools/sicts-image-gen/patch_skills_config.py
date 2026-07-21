#!/usr/bin/env python3
"""Patch Codex config.toml for sicts-imagegen (safe: skills.config only).

Windows / TOML note
-------------------
Double-quoted TOML strings process escapes. Writing:

    path = "C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md"

is parsed as a ``\\U`` unicode escape and breaks Codex with:

    too few unicode value digits, expected unicode hexadecimal value

Customer-validated fix: **TOML literal (single-quoted) strings** keep
backslashes as-is:

    path = 'C:\\Users\\73666\\.codex\\skills\\sicts-imagegen\\SKILL.md'

(shown in config as path = 'C:\\Users\\...' with real backslashes)

Unix paths stay single-quoted with forward slashes.
"""
from __future__ import annotations

import re
import sys
from datetime import date
from pathlib import Path


def is_windows_path(path: Path | str) -> bool:
    s = str(path)
    return bool(re.match(r"^[A-Za-z]:[/\\]", s)) or ("\\" in s and not s.startswith("/"))


def toml_path_body(path: Path | str) -> str:
    """Path body for a TOML single-quoted (literal) string — no quote marks."""
    p = Path(path).expanduser()
    # Prefer resolve when the path exists; fall back to as-given for markers.
    try:
        s = str(p.resolve()) if p.exists() else str(p)
    except OSError:
        s = str(p)

    if is_windows_path(s) or is_windows_path(path):
        # Native Windows separators inside a literal string.
        s = s.replace("/", "\\")
    else:
        s = Path(s).as_posix().replace("\\", "/")

    # TOML literal string: only escape is doubling single quotes.
    return s.replace("'", "''")


def path_assignment(path: Path | str) -> str:
    """Full TOML assignment line using single quotes (literal string)."""
    return f"path = '{toml_path_body(path)}'"


def path_markers(skill_path: Path) -> list[str]:
    """Markers used to find an existing [[skills.config]] block for this skill."""
    resolved = skill_path.expanduser()
    try:
        if resolved.exists():
            resolved = resolved.resolve()
    except OSError:
        pass

    variants = {
        str(resolved),
        resolved.as_posix(),
        str(resolved).replace("\\", "/"),
        str(resolved).replace("/", "\\"),
        str(skill_path),
        Path(skill_path).as_posix(),
        str(skill_path).replace("\\", "/"),
        str(skill_path).replace("/", "\\"),
        toml_path_body(resolved),
        toml_path_body(skill_path),
    }
    s = resolved.as_posix()
    if "sicts-imagegen" in s:
        variants.add("sicts-imagegen/SKILL.md")
        variants.add("sicts-imagegen\\SKILL.md")
    if "imagegen" in s and ".system" in s:
        variants.add(".system/imagegen/SKILL.md")
        variants.add(".system\\imagegen\\SKILL.md")
        variants.add("skills/.system/imagegen/SKILL.md")
        variants.add("skills\\.system\\imagegen\\SKILL.md")
    return [v for v in variants if v]


def ensure_block(text: str, skill_path: Path, enabled: bool, comment: str) -> str:
    alt = path_markers(skill_path)
    en = "true" if enabled else "false"
    path_line = path_assignment(skill_path)
    en_line = f"enabled = {en}"

    lines = text.splitlines()
    i = 0
    while i < len(lines):
        if lines[i].strip() == "[[skills.config]]":
            start = i
            j = i + 1
            block: list[str] = []
            while j < len(lines) and not lines[j].strip().startswith("["):
                block.append(lines[j])
                j += 1
            blob = "\n".join(block)
            if any(m in blob for m in alt):
                new_block: list[str] = []
                saw_path = False
                saw_en = False
                for bl in block:
                    if bl.strip().startswith("path"):
                        new_block.append(path_line)
                        saw_path = True
                    elif bl.strip().startswith("enabled"):
                        new_block.append(en_line)
                        saw_en = True
                    else:
                        new_block.append(bl)
                if not saw_path:
                    new_block.insert(0, path_line)
                if not saw_en:
                    new_block.append(en_line)
                lines = lines[: start + 1] + new_block + lines[j:]
                out = "\n".join(lines)
                if not out.endswith("\n"):
                    out += "\n"
                return out
            i = j
            continue
        i += 1

    add = (
        f"\n# {comment}\n"
        f"[[skills.config]]\n"
        f"{path_line}\n"
        f"{en_line}\n"
    )
    return text.rstrip("\n") + "\n" + add + "\n"


# Match path = "..." or path = '...' (quote may be unbalanced if file is broken).
_PATH_LINE_RE = re.compile(
    r"""^(\s*path\s*=\s*)(?P<q>["'])(?P<body>.*)(?P=q)(\s*(?:#.*)?)?$"""
)
# Broken double-quoted Windows path where closing quote still exists but body has \.
_PATH_BROKEN_DQ_RE = re.compile(
    r"""^(\s*path\s*=\s*)"(?P<body>.*)"(\s*(?:#.*)?)?$"""
)


def _normalize_path_body(body: str) -> str:
    """Turn a raw path body (possibly double-escaped) into filesystem form."""
    cleaned = (
        body.replace("\\\\", "\\")
        .replace("\\/", "/")
        .replace('\\"', '"')
        .replace("''", "'")
    )
    return cleaned


def repair_windows_path_lines(text: str) -> tuple[str, int]:
    """Rewrite unsafe Windows path lines to single-quoted literal form.

    Targets:
      path = "C:\\Users\\..."   (broken TOML — \\U unicode escape)
      path = "C:/Users/..."     (works, but normalize to single-quote style on Windows)
      path = 'C:/Users/...'     (works; leave unless mixed style with backslashes needed)

    Result (Windows):
      path = 'C:\\Users\\...\\SKILL.md'
    """
    fixed = 0
    out_lines: list[str] = []
    for line in text.splitlines():
        m = _PATH_LINE_RE.match(line) or _PATH_BROKEN_DQ_RE.match(line)
        if not m:
            out_lines.append(line)
            continue

        q = m.groupdict().get("q") or '"'
        body = m.group("body")
        suffix = ""
        # group index for trailing comment differs between regexes
        if m.lastindex and m.lastindex >= 3:
            # for _PATH_LINE_RE: groups 1=prefix, q, body, 4=suffix
            # for _PATH_BROKEN_DQ_RE: 1=prefix, body, 3=suffix
            try:
                suffix = m.group(4) or ""
            except IndexError:
                suffix = m.group(3) or "" if m.lastindex >= 3 else ""

        raw = _normalize_path_body(body)
        looks_win = is_windows_path(raw) or ("\\" in body)
        # Only rewrite when double-quoted with backslashes (broken/unsafe),
        # or double-quoted Windows drive path (normalize to customer-proven form).
        needs = False
        if q == '"' and ("\\" in body or looks_win):
            needs = True
        elif q == "'" and looks_win and "/" in raw and "\\" not in raw:
            # optional: single-quoted forward-slash Windows path → backslash form
            needs = True

        if not needs:
            out_lines.append(line)
            continue

        if looks_win or "\\" in raw:
            cleaned = raw.replace("/", "\\")
        else:
            cleaned = raw.replace("\\", "/")
        cleaned = cleaned.replace("'", "''")
        prefix = m.group(1)
        out_lines.append(f"{prefix}'{cleaned}'{suffix}")
        fixed += 1

    out = "\n".join(out_lines)
    if text.endswith("\n"):
        out += "\n"
    elif out and not out.endswith("\n"):
        out += "\n"
    return out, fixed


def main() -> int:
    # Modes:
    #   patch_skills_config.py <config.toml> <sicts SKILL.md> <CODEX_HOME> <disable_system 0|1>
    #   patch_skills_config.py --repair <config.toml>
    if len(sys.argv) >= 2 and sys.argv[1] in ("--repair", "repair"):
        if len(sys.argv) != 3:
            print(
                "usage: patch_skills_config.py --repair <config.toml>",
                file=sys.stderr,
            )
            return 2
        cfg_path = Path(sys.argv[2])
        if not cfg_path.exists():
            print(f"config not found: {cfg_path}", file=sys.stderr)
            return 1
        text = cfg_path.read_text(encoding="utf-8")
        fixed_text, n = repair_windows_path_lines(text)
        if n:
            cfg_path.write_text(fixed_text, encoding="utf-8")
            print(f"==> repaired {n} path line(s) in {cfg_path}")
        else:
            print(f"==> no unsafe path lines found in {cfg_path}")
        return 0

    if len(sys.argv) != 5:
        print(
            "usage: patch_skills_config.py <config.toml> <sicts SKILL.md> <CODEX_HOME> <disable_system 0|1>\n"
            "       patch_skills_config.py --repair <config.toml>",
            file=sys.stderr,
        )
        return 2

    cfg_path = Path(sys.argv[1])
    sicts_skill = Path(sys.argv[2]).expanduser().resolve()
    codex_home = Path(sys.argv[3]).expanduser().resolve()
    disable_system = sys.argv[4] not in ("0", "false", "False", "no", "NO")
    system_skill = (codex_home / "skills" / ".system" / "imagegen" / "SKILL.md").resolve()

    text = cfg_path.read_text(encoding="utf-8") if cfg_path.exists() else ""
    # Always try to heal any pre-existing broken Windows path lines first.
    text, repaired = repair_windows_path_lines(text)
    before = text

    text = ensure_block(
        text,
        sicts_skill,
        True,
        f"sicts-imagegen installer {date.today().isoformat()}",
    )
    if disable_system:
        text = ensure_block(
            text,
            system_skill,
            False,
            "disable system imagegen so sicts-imagegen is preferred",
        )

    if text != before or repaired:
        cfg_path.parent.mkdir(parents=True, exist_ok=True)
        cfg_path.write_text(text if text.endswith("\n") else text + "\n", encoding="utf-8")
        if repaired:
            print(f"==> repaired {repaired} path line(s) and updated skills.config in {cfg_path}")
        else:
            print(f"==> updated skills.config in {cfg_path}")
    else:
        print("==> skills.config already up to date")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
