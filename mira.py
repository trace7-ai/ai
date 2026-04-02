#!/usr/bin/env python3
"""
Mira CLI V1.1 — 字节内部 AI 编程助手（纯自研，零外部依赖）

完整版：直接调 Mira API，支持流式对话、MCP 工具链（飞书/搜索/画图/Memory）、
对话持久化、长期记忆、模型切换、本地工具执行。
"""

import sys

if len(sys.argv) >= 2 and sys.argv[1] == "ask":
    from mira_sidecar_main import main as _sidecar_main

    sys.exit(_sidecar_main(sys.argv[2:]))

import json
import os
import re
import readline
import select
import shutil
import subprocess
import time
import threading
import uuid
import hashlib
import urllib.request
import urllib.error
import urllib.parse
import http.client
import ssl
import difflib
import webbrowser
from datetime import datetime
from pathlib import Path
from typing import Optional

# ============================================================================
# 常量
# ============================================================================

VERSION = "1.0"

MIRA_HOME = Path(os.environ.get("MIRA_HOME", Path.home() / ".mira"))
CONFIG_FILE = MIRA_HOME / "config.json"
HISTORY_FILE = MIRA_HOME / "history"
MODEL_FILE = MIRA_HOME / "model"
CONV_DIR = MIRA_HOME / "conversations"
LAST_CONV_FILE = MIRA_HOME / "last_conversation"

MIRA_BASE = "https://mira.byteintl.net"
MCP_URL = f"{MIRA_BASE}/mira/proxy/mcp"
OAUTH_APP_ID = "cli_a8612af21878900c"

# 更新源（TOS 或飞书云盘，安装后可配置）
UPDATE_URL_FILE = MIRA_HOME / "update_url"
DEFAULT_UPDATE_URL = "https://tosv.byted.org/obj/juren-cn/Mira-Cli/mira.py"
MAX_TOKENS = 16384
MAX_HISTORY_MESSAGES = 200

# ─── 模型 ───
# (display_name, api_model_id, thinking_mode)
# thinking_mode: "quick" = normal, "deep" = extended thinking
MODELS = {
    # Claude 系列
    "opus4.6":    ("Cloud-O-4.6",       "re-o-46",                       "quick"),
    "opus4.6t":   ("Cloud-O-4.6 Think", "re-o-46",                       "deep"),
    "opus4.5":    ("Cloud-O-4.5",       "re-o-45",                       "quick"),
    "sonnet4.6":  ("Cloud-S-4.6",       "re-s-46",                       "quick"),
    "sonnet4":    ("Cloud-S-4",         "claude-sonnet-4-20250514",      "quick"),
    "sonnet3.7":  ("Cloud-S-3.7",       "claude-3-7-sonnet-20250219",    "quick"),
    "sonnet3.5":  ("Cloud-S-3.5",       "claude-3-5-sonnet-20241022",    "quick"),
    "haiku3.5":   ("Cloud-H-3.5",       "claude-3-5-haiku-20241022",     "quick"),
    # External Flagship Models
    "gpt5.4":     ("GPT-5.4",           "gpt-5.4",                      "quick"),
    "gemini3.1":  ("Gemini 3.1 Pro",    "gemini-3.1-pro-preview",       "quick"),
    "glm5":       ("Glm-5",             "glm-5",                        "quick"),
}
MODEL_ALIASES = {
    "opus46": "opus4.6", "o46": "opus4.6", "opus": "opus4.6", "cloud-o-4.6": "opus4.6",
    "opus46t": "opus4.6t", "think": "opus4.6t",
    "opus45": "opus4.5", "o45": "opus4.5",
    "sonnet46": "sonnet4.6", "s46": "sonnet4.6", "cloud-s-4.6": "sonnet4.6",
    "sonnet": "sonnet4", "s4": "sonnet4",
    "sonnet37": "sonnet3.7", "s37": "sonnet3.7",
    "sonnet35": "sonnet3.5", "s35": "sonnet3.5",
    "haiku": "haiku3.5", "h": "haiku3.5", "haiku35": "haiku3.5",
    "gpt": "gpt5.4", "gpt54": "gpt5.4",
    "gemini": "gemini3.1", "gemini31": "gemini3.1",
    "glm": "glm5",
}
DEFAULT_MODEL = "opus4.6"

# ─── Client Type (Coco / Trae / Web 兼容) ───


# ─── ANSI (256-color, Coco-style palette) ───
class C:
    RESET   = "\033[0m"
    BOLD    = "\033[1m"
    DIM     = "\033[2m"
    # ─── Coco 256-color palette ───
    ORANGE  = "\033[38;5;174m"   # 主色调 — 工具名、加粗文本
    GRAY    = "\033[38;5;246m"   # 次要信息 — 参数、dim 内容
    GREEN   = "\033[38;5;114m"   # 成功状态 — prompt 箭头、bullet
    CYAN    = "\033[38;5;153m"   # 高亮 — 行内代码、代码块
    PURPLE  = "\033[38;5;141m"   # 标题
    RED     = "\033[38;5;203m"   # 错误、spinner 标记
    YELLOW  = "\033[38;5;222m"   # 警告
    BLUE    = "\033[38;5;111m"   # 链接、辅助
    WHITE   = "\033[38;5;231m"   # 正文高亮
    DIVIDER = "\033[38;5;239m"   # 分隔线、边框
    MAGENTA = "\033[38;5;141m"   # alias → PURPLE
    # ─── 光标控制 ───
    CURSOR_HIDE = "\033[?25l"
    CURSOR_SHOW = "\033[?25h"

def term_width(): return shutil.get_terminal_size((80, 24)).columns

# ─── readline prompt ANSI 安全包裹 ───
_ANSI_RE = re.compile(r'(\033\[[0-9;]*[A-Za-z])')

def _rl_prompt(raw: str) -> str:
    """将 prompt 字符串中的 ANSI 转义码用 \\x01/\\x02 包裹，
    使 readline 正确计算可见宽度，避免上下翻历史时光标错位、旧文本删不掉。"""
    return _ANSI_RE.sub(lambda m: f"\x01{m.group(1)}\x02", raw)

import unicodedata as _ucd

def _display_width(s: str) -> int:
    """计算字符串在终端中的显示宽度（考虑 CJK 宽字符，忽略 ANSI 转义码）"""
    s = _ANSI_RE.sub('', s)
    w = 0
    for ch in s:
        cat = _ucd.east_asian_width(ch)
        w += 2 if cat in ('W', 'F') else 1
    return w

def _pad_to_width(s: str, target: int) -> str:
    """在字符串右侧补空格使其显示宽度达到 target"""
    cur = _display_width(s)
    return s + ' ' * max(0, target - cur)

def _render_inline(text: str) -> str:
    """渲染一行文本中的行内 Markdown（**加粗**、`代码`、*斜体*）"""
    # `code` → cyan
    text = re.sub(r'`([^`]+)`', lambda m: f'{C.CYAN}{m.group(1)}{C.RESET}', text)
    # **bold** → Coco orange + bold
    text = re.sub(r'\*\*(.+?)\*\*', lambda m: f'{C.BOLD}{C.ORANGE}{m.group(1)}{C.RESET}', text)
    # *italic* (单星号，排除 **)
    text = re.sub(r'(?<!\*)\*([^\*]+?)\*(?!\*)', lambda m: f'{C.DIM}{m.group(1)}{C.RESET}', text)
    # [text](url) → text + dim url
    text = re.sub(r'\[([^\]]+)\]\(([^)]+)\)', lambda m: f'{C.BLUE}{m.group(1)}{C.RESET}{C.DIM}({m.group(2)}){C.RESET}', text)
    return text

# ─── 左边距常量 ───
_INDENT = "  "  # 所有输出行统一 2 空格左边距

class _MarkdownStreamer:
    """流式 Markdown 渲染器 v3 — Coco 风格，统一 2 空格左边距"""

    def __init__(self):
        self._buf = ""
        self._in_code_block = False
        self._code_lang = ""
        self._table_rows = []  # 表格行缓冲区

    def _flush_table(self) -> str:
        """将缓冲的表格行渲染成对齐的表格"""
        if not self._table_rows:
            return ""
        rows = self._table_rows
        self._table_rows = []

        # 解析所有行的单元格
        parsed = []
        sep_indices = []
        for i, row in enumerate(rows):
            s = row.strip()
            cells = [c.strip() for c in s.strip("|").split("|")]
            if re.match(r'^[\|\s:\-]+$', s):
                sep_indices.append(i)
                parsed.append(cells)
            else:
                parsed.append(cells)

        # 统一列数
        ncols = max(len(r) for r in parsed) if parsed else 0
        for r in parsed:
            while len(r) < ncols:
                r.append("")

        # 计算每列最大显示宽度（考虑 CJK 宽字符）
        col_widths = [0] * ncols
        for i, cells in enumerate(parsed):
            if i in sep_indices:
                continue
            for j, c in enumerate(cells):
                col_widths[j] = max(col_widths[j], _display_width(c))

        # 按内容比例分配列宽（内容多的列分到更多空间）
        # 可用总宽度 = 终端宽度 - 左边距(2) - 左边框(1) - 右边框(1) - 列间分隔(ncols-1) - 每列左右padding(ncols*2)
        avail = term_width() - 4 - (ncols - 1) - ncols * 2
        total_content = sum(col_widths) or 1
        if total_content <= avail:
            pass  # 所有列都放得下，不需要压缩
        else:
            # 按比例分配，但保证每列至少 min_col 宽
            min_col = max(4, avail // (ncols * 3))  # 最小列宽
            # 先给每列分配最小宽度，剩余按比例分配
            remaining = avail - min_col * ncols
            if remaining > 0:
                new_widths = []
                for w in col_widths:
                    extra = int(remaining * w / total_content)
                    new_widths.append(min_col + extra)
                # 修正舍入误差：把剩余宽度加给最宽的列
                diff = avail - sum(new_widths)
                if diff != 0:
                    widest = new_widths.index(max(new_widths))
                    new_widths[widest] += diff
                col_widths = new_widths
            else:
                col_widths = [max(min_col, 4)] * ncols

        H = self._H
        out_lines = []

        # 顶部边框
        top_sep = f"{C.DIVIDER}┬{C.RESET}".join(
            f"{C.DIVIDER}{H * (w + 2)}{C.RESET}" for w in col_widths)
        out_lines.append(f"{_INDENT}{C.DIVIDER}┌{C.RESET}{top_sep}{C.DIVIDER}┐{C.RESET}")

        for i, cells in enumerate(parsed):
            if i in sep_indices:
                # 分隔行
                mid_sep = f"{C.DIVIDER}┼{C.RESET}".join(
                    f"{C.DIVIDER}{H * (w + 2)}{C.RESET}" for w in col_widths)
                out_lines.append(f"{_INDENT}{C.DIVIDER}├{C.RESET}{mid_sep}{C.DIVIDER}┤{C.RESET}")
            else:
                # 数据行（支持单元格内折行）
                # 1. 先把每个单元格内容按列宽拆成多行
                cell_lines = []  # cell_lines[j] = ["line1", "line2", ...]
                for j, c in enumerate(cells):
                    w = col_widths[j] if j < len(col_widths) else 4
                    cw = _display_width(c)
                    if cw <= w:
                        cell_lines.append([c])
                    else:
                        # 按显示宽度折行
                        lines = []
                        while c:
                            chunk = ""
                            chunk_w = 0
                            remaining = c
                            for ci, ch in enumerate(remaining):
                                chw = 2 if _ucd.east_asian_width(ch) in ('W', 'F') else 1
                                if chunk_w + chw > w:
                                    break
                                chunk += ch
                                chunk_w += chw
                            if not chunk:
                                chunk = remaining[0]
                            lines.append(chunk)
                            c = c[len(chunk):]
                        cell_lines.append(lines)

                # 2. 确定该行最大子行数
                max_sub = max(len(cl) for cl in cell_lines) if cell_lines else 1

                # 3. 逐子行渲染
                for sub_idx in range(max_sub):
                    parts = []
                    for j in range(ncols):
                        w = col_widths[j] if j < len(col_widths) else 4
                        cl = cell_lines[j] if j < len(cell_lines) else [""]
                        text = cl[sub_idx] if sub_idx < len(cl) else ""
                        rendered = _render_inline(text)
                        vis_w = _display_width(rendered)
                        pad = max(0, w - vis_w)
                        parts.append(f" {rendered}{' ' * pad} ")
                    row_str = f"{C.DIVIDER}│{C.RESET}".join(parts)
                    out_lines.append(f"{_INDENT}{C.DIVIDER}│{C.RESET}{row_str}{C.DIVIDER}│{C.RESET}")

        # 底部边框
        bot_sep = f"{C.DIVIDER}┴{C.RESET}".join(
            f"{C.DIVIDER}{H * (w + 2)}{C.RESET}" for w in col_widths)
        out_lines.append(f"{_INDENT}{C.DIVIDER}└{C.RESET}{bot_sep}{C.DIVIDER}┘{C.RESET}")

        return "\n".join(out_lines)

    def feed(self, text: str) -> str:
        self._buf += text
        out = ""
        while "\n" in self._buf:
            line, self._buf = self._buf.split("\n", 1)
            stripped = line.strip()
            # 检查是否为表格行
            if not self._in_code_block and stripped.startswith("|") and stripped.endswith("|"):
                self._table_rows.append(line)
                continue
            # 非表格行 → 先刷出缓冲的表格
            if self._table_rows:
                out += self._flush_table() + "\n"
            out += self._render_line(line) + "\n"
        return out

    def finish(self) -> str:
        out = ""
        # 先刷出缓冲的表格
        if self._table_rows:
            out += self._flush_table() + "\n"
        if self._buf:
            out += self._render_line(self._buf)
            self._buf = ""
        if self._in_code_block:
            self._in_code_block = False
            self._code_lang = ""
        return out

    _H = "─"  # box-drawing horizontal bar (extracted for f-string compat)

    def _render_line(self, line: str) -> str:
        stripped = line.strip()
        H = self._H

        # ─── 代码块 toggle ───
        if stripped.startswith("```"):
            # box_w: ┌/└ 和 ┐/┘ 之间的横线字符数
            # 终端总占: indent(2) + ┌(1) + ─*box_w + ┐(1) = W → box_w = W - 4
            box_w = max(20, term_width() - 4)
            if not self._in_code_block:
                self._in_code_block = True
                self._code_lang = stripped[3:].strip()
                if self._code_lang:
                    label = f" {self._code_lang} "
                    label_w = _display_width(label)
                    # ┌── label ───...───┐ 中横线数 = 2(──) + bar_rest
                    bar_rest = max(0, box_w - 2 - label_w)
                    return f"{_INDENT}{C.DIVIDER}┌──{label}{H * bar_rest}┐{C.RESET}"
                else:
                    return f"{_INDENT}{C.DIVIDER}┌{H * box_w}┐{C.RESET}"
            else:
                self._in_code_block = False
                self._code_lang = ""
                return f"{_INDENT}{C.DIVIDER}└{H * box_w}┘{C.RESET}"

        if self._in_code_block:
            # 内容行: │ text_pad │  (两侧各 1 space)
            # 可见: ┃(1) + space(1) + content + pad + space(1) + ┃(1) = usable + 4
            # 要等于 box_w + 2 (即 ┌+box_w+┐ 的宽度), 所以: usable + 4 = box_w + 2
            # → usable = box_w - 2 = W - 6
            usable = max(16, term_width() - 6)

            def _code_line(text):
                """渲染单行代码，带左右边框对齐"""
                vis_len = _display_width(text)
                pad = max(0, usable - vis_len)
                return f"{_INDENT}{C.DIVIDER}│{C.RESET} {C.CYAN}{text}{C.RESET}{' ' * pad} {C.DIVIDER}│{C.RESET}"

            if _display_width(line) <= usable:
                return _code_line(line)
            # 折行：按可见宽度逐段截取
            out_parts = []
            pos = 0
            while pos < len(line):
                # 逐字符取到 usable 宽度
                chunk = ""
                cw = 0
                for ch in line[pos:]:
                    chw = 2 if _ucd.east_asian_width(ch) in ('W', 'F') else 1
                    if cw + chw > usable:
                        break
                    chunk += ch
                    cw += chw
                if not chunk:  # 安全兜底：至少取一个字符
                    chunk = line[pos]
                pos += len(chunk)
                out_parts.append(_code_line(chunk))
            return "\n".join(out_parts)

        # ─── 空行 ───
        if not stripped:
            return ""

        # ─── 分隔线 ---/***/___ ───
        if re.match(r'^[-*_]{3,}\s*$', stripped):
            w = max(20, term_width() - 4)
            return f"{_INDENT}{C.DIVIDER}{H * w}{C.RESET}"

        # ─── 标题 # ## ### #### → Coco purple ───
        m = re.match(r'^(#{1,6})\s+(.+)', stripped)
        if m:
            level = len(m.group(1))
            title = _render_inline(m.group(2))
            if level <= 2:
                return f"\n{_INDENT}{C.BOLD}{C.PURPLE}{title}{C.RESET}"
            else:
                return f"\n{_INDENT}{C.PURPLE}{title}{C.RESET}"

        # ─── 引用 > ───
        if stripped.startswith("> "):
            content = _render_inline(stripped[2:])
            return f"{_INDENT}{C.DIVIDER}│{C.RESET} {C.GRAY}{content}{C.RESET}"
        if stripped == ">":
            return f"{_INDENT}{C.DIVIDER}│{C.RESET}"

        # ─── 表格行 | ... | ─── (fallback，正常由 feed() 拦截)
        if stripped.startswith("|") and stripped.endswith("|"):
            cells = [c.strip() for c in stripped.strip("|").split("|")]
            rendered = f" {C.DIVIDER}│{C.RESET} ".join(
                _render_inline(c) for c in cells)
            return f"{_INDENT}{C.DIVIDER}│{C.RESET} {rendered} {C.DIVIDER}│{C.RESET}"

        # ─── 无序列表 - / * ───
        m = re.match(r'^(\s*)([-*])\s+(.+)', line)
        if m:
            depth = len(m.group(1)) // 2
            content = _render_inline(m.group(3))
            pad = _INDENT + "  " * depth
            return f"{pad}{C.GRAY}•{C.RESET} {content}"

        # ─── 有序列表 1. / 2. ───
        m = re.match(r'^(\s*)(\d+)\.\s+(.+)', line)
        if m:
            depth = len(m.group(1)) // 2
            num = m.group(2)
            content = _render_inline(m.group(3))
            pad = _INDENT + "  " * depth
            return f"{pad}{C.GRAY}{num}.{C.RESET} {content}"

        # ─── 普通文本（带自动折行）───
        rendered = _render_inline(stripped)
        max_text = max(20, term_width() - 3)  # 2 indent + 1 safety
        # 计算无 ANSI 的显示宽度（正确处理 CJK 宽字符）
        vis_w = _display_width(rendered)
        if vis_w <= max_text:
            return f"{_INDENT}{rendered}"
        # 需要折行 — 按显示宽度在词边界折行（CJK 字符视为天然断点）
        out_lines = []
        cur_line = ""
        cur_w = 0
        for ch in stripped:
            chw = 2 if _ucd.east_asian_width(ch) in ('W', 'F') else 1
            # 在空格处可断行；CJK 字符前后也可断行
            if cur_w + chw > max_text and cur_line:
                out_lines.append(cur_line)
                cur_line = ""
                cur_w = 0
                if ch == ' ':
                    continue  # 跳过断行处的空格
            cur_line += ch
            cur_w += chw
        if cur_line:
            out_lines.append(cur_line)
        return "\n".join(f"{_INDENT}{_render_inline(wl)}" for wl in out_lines)

def _tool_summary(name: str, inp: dict) -> str:
    """生成工具调用的简短摘要（返回完整内容，由调用方负责折行显示）"""
    n = name.lower().replace("mira_local_", "")
    if n in ("bash", "execute_command"):
        cmd = inp.get("command", "")
        if not cmd:
            return ""
        # 将多行命令压缩为单行显示
        cmd = cmd.replace("\n", " ; ").strip()
        return cmd
    if n in ("read", "read_file"):
        p = inp.get("path", inp.get("file_path", ""))
        return p if p else ""
    if n in ("edit", "edit_file"):
        p = inp.get("path", inp.get("file_path", ""))
        return p if p else ""
    if n in ("write", "write_file"):
        p = inp.get("path", inp.get("file_path", ""))
        return p if p else ""
    if n in ("glob", "find_files"):
        return inp.get("pattern", "")
    if n in ("grep", "search"):
        return inp.get("pattern", inp.get("query", ""))
    # 通用：取第一个短字符串
    for v in inp.values():
        if isinstance(v, str) and 0 < len(v) <= 200:
            return v[:200]
    return ""

def ok(msg):   print(f"  {C.GREEN}✓{C.RESET} {msg}")
def warn(msg): print(f"  {C.YELLOW}⚠{C.RESET} {msg}")
def err(msg):  print(f"  {C.RED}✗{C.RESET} {msg}")
def dim(msg):  print(f"  {C.DIM}{msg}{C.RESET}")

def _safe_cookie(val: str) -> str:
    """Sanitize cookie string for HTTP header use.
    urllib encodes headers as latin-1 and will crash on non-latin-1 chars.
    Also strip control characters that can corrupt HTTP requests."""
    if not val:
        return ""
    # Remove control characters (0x00-0x1F, 0x7F) except space
    cleaned = "".join(c for c in val if ord(c) >= 0x20 and ord(c) != 0x7F)
    # Encode to ASCII, dropping anything that's not ASCII
    cleaned = cleaned.encode("ascii", errors="ignore").decode("ascii")
    return cleaned.strip()


def _http_request(url, method="GET", headers=None, data=None, timeout=30):
    """Make HTTP request using http.client for exact header control.
    urllib.request mangles header capitalization which some servers reject.
    Returns (status_code, response_body_bytes, response_headers)."""
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    path = parsed.path
    if parsed.query:
        path += "?" + parsed.query

    if parsed.scheme == "https":
        ctx = ssl.create_default_context()
        conn = http.client.HTTPSConnection(host, port, timeout=timeout, context=ctx)
    else:
        conn = http.client.HTTPConnection(host, port, timeout=timeout)

    try:
        conn.request(method, path, body=data, headers=headers or {})
        resp = conn.getresponse()
        body = resp.read()
        return resp.status, body, dict(resp.getheaders())
    finally:
        conn.close()


# ============================================================================
# 配置
# ============================================================================

class Config:
    def __init__(self):
        self.cookies = ""  # full cookie string: "_session_id=...; session=...; ..."
        self.username = ""
        self.device_id = ""  # MCP bridge device-id
        self.model_key = DEFAULT_MODEL
        self._load()

    def _load(self):
        if CONFIG_FILE.exists():
            try:
                d = json.loads(CONFIG_FILE.read_text())
                self.cookies = d.get("cookies", "")
                # Migration from old config format
                if not self.cookies and d.get("session"):
                    self.cookies = f"session={d['session']}"
                self.username = d.get("username", "")
                self.device_id = d.get("device_id", "")
            except Exception:
                pass
        if MODEL_FILE.exists():
            k = MODEL_FILE.read_text().strip()
            if k in MODELS: self.model_key = k

    def save(self):
        MIRA_HOME.mkdir(parents=True, exist_ok=True)
        CONFIG_FILE.write_text(json.dumps({
            "cookies": self.cookies, "username": self.username,
            "device_id": self.device_id,
        }, indent=2))
        CONFIG_FILE.chmod(0o600)

    def save_model(self):
        MIRA_HOME.mkdir(parents=True, exist_ok=True)
        MODEL_FILE.write_text(self.model_key)
        MODEL_FILE.chmod(0o600)

    @property
    def model_id(self): return MODELS[self.model_key][1]
    @property
    def model_name(self): return MODELS[self.model_key][0]
    @property
    def model_mode(self): return MODELS[self.model_key][2]  # "quick" or "deep"
    @property
    def has_auth(self): return bool(self.cookies)
    @property
    def has_mcp(self): return bool(self.cookies)

    def _extract_username(self):
        """尝试从 cookie JWT 中提取用户名"""
        if self.username:
            return
        import base64
        for part in self.cookies.split(";"):
            part = part.strip()
            if "=" not in part:
                continue
            name, val = part.split("=", 1)
            name = name.strip()
            if name not in ("mira_session", "bd_sso_3b6da9"):
                continue
            # JWT: header.payload.signature
            segments = val.strip().split(".")
            if len(segments) < 2:
                continue
            try:
                payload_b64 = segments[1]
                # 补齐 base64 padding
                payload_b64 += "=" * (4 - len(payload_b64) % 4)
                decoded = base64.urlsafe_b64decode(payload_b64)
                data = json.loads(decoded)
                # 常见字段：name, preferred_username, email, sub
                for field in ("name", "preferred_username", "email", "sub"):
                    v = data.get(field, "")
                    if v and "@" not in v and len(v) < 40:
                        self.username = v
                        return
                # email fallback: 取 @ 前面部分
                email = data.get("email", "")
                if email and "@" in email:
                    self.username = email.split("@")[0]
                    return
            except Exception:
                continue






# ============================================================================
# 浏览器 Cookie 读取（零外部依赖）
# ============================================================================

_cookie_debug = os.environ.get("MIRA_DEBUG", "") == "1"

def _cdebug(msg):
    if _cookie_debug:
        print(f"  {C.DIM}[cookie] {msg}{C.RESET}", file=sys.stderr)


def _find_browser_cookies():
    """查找本地浏览器 cookie 数据库路径"""
    import glob
    home = str(Path.home())
    is_mac = sys.platform == "darwin"
    is_linux = sys.platform.startswith("linux")
    results = []

    def _chrome_profiles(base_dir):
        found = []
        if not os.path.isdir(base_dir):
            return found
        candidates = ["Default"]
        for entry in sorted(os.listdir(base_dir)):
            if entry.startswith("Profile ") and os.path.isdir(os.path.join(base_dir, entry)):
                candidates.append(entry)
        for prof in candidates:
            for sub in ["Network/Cookies", "Cookies"]:
                p = os.path.join(base_dir, prof, sub)
                if os.path.isfile(p):
                    found.append(p)
                    break
        return found

    if is_mac:
        for name, base in [
            ("Chrome", f"{home}/Library/Application Support/Google/Chrome"),
            ("Edge", f"{home}/Library/Application Support/Microsoft Edge"),
            ("Chromium", f"{home}/Library/Application Support/Chromium"),
        ]:
            for p in _chrome_profiles(base):
                results.append((name, p, "chrome"))
        for prof in glob.glob(f"{home}/Library/Application Support/Firefox/Profiles/*.default*"):
            p = os.path.join(prof, "cookies.sqlite")
            if os.path.isfile(p):
                results.append(("Firefox", p, "firefox"))
                break
        p = f"{home}/Library/Cookies/Cookies.binarycookies"
        if os.path.isfile(p) and os.access(p, os.R_OK):
            results.append(("Safari", p, "safari"))
    elif is_linux:
        for name, base in [
            ("Chrome", f"{home}/.config/google-chrome"),
            ("Edge", f"{home}/.config/microsoft-edge"),
            ("Chromium", f"{home}/.config/chromium"),
        ]:
            for p in _chrome_profiles(base):
                results.append((name, p, "chrome"))
        for prof in glob.glob(f"{home}/.mozilla/firefox/*.default*"):
            p = os.path.join(prof, "cookies.sqlite")
            if os.path.isfile(p):
                results.append(("Firefox", p, "firefox"))
                break

    _cdebug(f"found {len(results)} browser(s)")
    return results


def _read_firefox_cookie(cookie_path, domain, name):
    """从 Firefox 读 cookie（明文）"""
    import sqlite3
    import tempfile
    fd, tmp = tempfile.mkstemp(suffix=".sqlite")
    os.close(fd)
    try:
        shutil.copy2(cookie_path, tmp)
        for ext in ["-wal", "-shm"]:
            src = cookie_path + ext
            if os.path.isfile(src):
                shutil.copy2(src, tmp + ext)
        conn = sqlite3.connect(tmp)
        conn.execute("PRAGMA journal_mode=wal")
        cur = conn.cursor()
        cur.execute(
            "SELECT value FROM moz_cookies WHERE host LIKE ? AND name = ?",
            (f"%{domain}", name),
        )
        row = cur.fetchone()
        conn.close()
        return row[0] if row else None
    except Exception as e:
        _cdebug(f"firefox error: {e}")
        return None
    finally:
        for f in [tmp, tmp + "-wal", tmp + "-shm"]:
            try:
                os.unlink(f)
            except OSError:
                pass


def _chrome_decrypt_value(encrypted_value, browser_name="Chrome"):
    """解密 Chrome/Edge/Chromium 的 cookie 值"""
    if not encrypted_value:
        return None
    prefix = encrypted_value[:3]
    if prefix not in (b"v10", b"v11"):
        try:
            return encrypted_value.decode("utf-8")
        except Exception:
            return None
    ciphertext = encrypted_value[3:]
    if not ciphertext:
        return None
    _cdebug(f"decrypt: prefix={prefix} ct_len={len(ciphertext)}")

    password = None
    iterations = 1

    if sys.platform == "darwin":
        service = {
            "Chrome": "Chrome Safe Storage",
            "Edge": "Microsoft Edge Safe Storage",
            "Chromium": "Chromium Safe Storage",
        }.get(browser_name, "Chrome Safe Storage")
        try:
            proc = subprocess.run(
                ["security", "find-generic-password", "-w", "-s", service],
                capture_output=True, text=True, timeout=5,
            )
            if proc.returncode == 0 and proc.stdout.strip():
                password = proc.stdout.strip().encode("utf-8")
                iterations = 1003
                _cdebug(f"keychain OK for '{service}'")
            else:
                _cdebug(f"keychain FAIL rc={proc.returncode}")
        except Exception as e:
            _cdebug(f"keychain error: {e}")
    else:
        for schema in [
            "chrome_libsecret_os_crypt_password_v2",
            "chrome_libsecret_os_crypt_password_v1",
        ]:
            try:
                app_name = browser_name.lower().replace(" ", "-")
                if app_name == "edge":
                    app_name = "microsoft-edge"
                proc = subprocess.run(
                    ["secret-tool", "lookup", "xdg:schema", schema, "application", app_name],
                    capture_output=True, text=True, timeout=5,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    password = proc.stdout.strip().encode("utf-8")
                    break
            except Exception:
                continue
        if not password:
            password = b"peanuts"
        iterations = 1

    if not password:
        return None

    key = hashlib.pbkdf2_hmac("sha1", password, b"saltysalt", iterations, dklen=16)
    iv = b" " * 16
    try:
        proc = subprocess.run(
            ["openssl", "enc", "-aes-128-cbc", "-d", "-K", key.hex(), "-iv", iv.hex(), "-nopad"],
            input=ciphertext, capture_output=True, timeout=5,
        )
        if proc.returncode != 0:
            _cdebug(f"openssl FAIL rc={proc.returncode}")
            return None
        plaintext = proc.stdout
        if not plaintext:
            return None
        pad_len = plaintext[-1]
        if 1 <= pad_len <= 16 and all(b == pad_len for b in plaintext[-pad_len:]):
            plaintext = plaintext[:-pad_len]
        # Validate: decrypted cookie values must be printable ASCII/UTF-8
        # If we get garbage bytes, the decryption key was wrong (e.g. App-Bound Encryption)
        try:
            result = plaintext.decode("utf-8")
        except UnicodeDecodeError:
            _cdebug(f"decrypt produced non-UTF-8 bytes, key likely wrong")
            return None
        # Check that result is mostly printable (JWT tokens, hex strings, etc.)
        if result and len(result) > 0:
            non_printable = sum(1 for ch in result if ord(ch) < 32 and ch not in '\t\n\r')
            if non_printable > len(result) * 0.1:  # >10% non-printable = garbage
                _cdebug(f"decrypt produced garbage ({non_printable}/{len(result)} non-printable)")
                return None
        _cdebug(f"decrypted len={len(result)}")
        return result
    except Exception as e:
        _cdebug(f"openssl error: {e}")
        return None


def _read_chrome_cookie(cookie_path, domain, name, browser_name="Chrome"):
    """从 Chrome 读并解密 cookie"""
    import sqlite3
    import tempfile
    fd, tmp = tempfile.mkstemp(suffix=".db")
    os.close(fd)
    try:
        shutil.copy2(cookie_path, tmp)
        # Chrome uses WAL mode — must copy WAL/SHM for recent cookies
        for ext in ["-wal", "-shm", "-journal"]:
            src = cookie_path + ext
            if os.path.isfile(src):
                try:
                    shutil.copy2(src, tmp + ext)
                except Exception:
                    pass
        conn = sqlite3.connect(f"file:{tmp}?mode=ro", uri=True)
        try:
            conn.execute("PRAGMA journal_mode=wal")
        except Exception:
            pass
        cur = conn.cursor()
        cur.execute(
            "SELECT value, encrypted_value FROM cookies "
            "WHERE host_key LIKE ? AND name = ?",
            (f"%{domain}%", name),
        )
        row = cur.fetchone()
        conn.close()
        if not row:
            return None
        value, enc_value = row
        if value:
            return value
        return _chrome_decrypt_value(enc_value, browser_name)
    except Exception as e:
        _cdebug(f"chrome read error: {e}")
        return None
    finally:
        for f_ext in ["", "-wal", "-shm", "-journal"]:
            try:
                os.unlink(tmp + f_ext)
            except OSError:
                pass


def _read_safari_cookie(cookie_path, domain, name):
    """从 Safari 读 cookie（二进制格式，明文）"""
    import struct
    try:
        with open(cookie_path, "rb") as f:
            magic = f.read(4)
            if magic != b"cook":
                return None
            num_pages = struct.unpack(">I", f.read(4))[0]
            page_sizes = [struct.unpack(">I", f.read(4))[0] for _ in range(num_pages)]
            for ps in page_sizes:
                page = f.read(ps)
                if len(page) < 8:
                    continue
                num_cookies = struct.unpack("<I", page[4:8])[0]
                offsets = []
                for i in range(num_cookies):
                    if 8 + i * 4 + 4 <= len(page):
                        offsets.append(struct.unpack("<I", page[8 + i * 4:12 + i * 4])[0])
                for off in offsets:
                    rec = page[off:]
                    if len(rec) < 56:
                        continue
                    url_off = struct.unpack("<I", rec[16:20])[0]
                    name_off = struct.unpack("<I", rec[20:24])[0]
                    val_off = struct.unpack("<I", rec[28:32])[0]
                    d = rec[url_off:].split(b"\x00")[0].decode("utf-8", errors="ignore")
                    n = rec[name_off:].split(b"\x00")[0].decode("utf-8", errors="ignore")
                    v = rec[val_off:].split(b"\x00")[0].decode("utf-8", errors="ignore")
                    if domain in d and n == name:
                        return v
    except PermissionError:
        pass  # macOS Safari 需要"完全磁盘访问"权限，静默跳过
    except Exception as e:
        _cdebug(f"safari error: {e}")
    return None


def _read_browser_cookie(bname, bpath, btype, domain, name):
    """统一入口：从指定浏览器读取一个 cookie"""
    if btype == "firefox":
        return _read_firefox_cookie(bpath, domain, name)
    elif btype == "chrome":
        return _read_chrome_cookie(bpath, domain, name, bname)
    elif btype == "safari":
        return _read_safari_cookie(bpath, domain, name)
    return None


def _read_all_chrome_cookies(cookie_path, domain, browser_name="Chrome"):
    """从 Chrome 读取指定域名下的所有 cookie，返回 [(name, value), ...]"""
    import sqlite3
    import tempfile
    fd, tmp = tempfile.mkstemp(suffix=".db")
    os.close(fd)
    try:
        shutil.copy2(cookie_path, tmp)
        # Chrome uses WAL mode — must copy WAL/SHM for recent cookies
        for ext in ["-wal", "-shm", "-journal"]:
            src = cookie_path + ext
            if os.path.isfile(src):
                try:
                    shutil.copy2(src, tmp + ext)
                except Exception:
                    pass
        conn = sqlite3.connect(f"file:{tmp}?mode=ro", uri=True)
        try:
            conn.execute("PRAGMA journal_mode=wal")
        except Exception:
            pass
        cur = conn.cursor()
        cur.execute(
            "SELECT name, value, encrypted_value FROM cookies "
            "WHERE host_key LIKE ?",
            (f"%{domain}%",),
        )
        results = []
        for name, value, enc_value in cur.fetchall():
            if value:
                results.append((name, value))
            else:
                dec = _chrome_decrypt_value(enc_value, browser_name)
                if dec:
                    results.append((name, dec))
        conn.close()
        return results
    except Exception as e:
        _cdebug(f"chrome read all error: {e}")
        return []
    finally:
        for f_ext in ["", "-wal", "-shm", "-journal"]:
            try: os.unlink(tmp + f_ext)
            except OSError: pass


def _read_all_firefox_cookies(cookie_path, domain):
    """从 Firefox 读取指定域名下的所有 cookie，返回 [(name, value), ...]"""
    import sqlite3
    import tempfile
    fd, tmp = tempfile.mkstemp(suffix=".sqlite")
    os.close(fd)
    try:
        shutil.copy2(cookie_path, tmp)
        for ext in ["-wal", "-shm"]:
            src = cookie_path + ext
            if os.path.isfile(src):
                shutil.copy2(src, tmp + ext)
        conn = sqlite3.connect(tmp)
        conn.execute("PRAGMA journal_mode=wal")
        cur = conn.cursor()
        cur.execute(
            "SELECT name, value FROM moz_cookies WHERE host LIKE ?",
            (f"%{domain}",),
        )
        results = [(name, value) for name, value in cur.fetchall()]
        conn.close()
        return results
    except Exception as e:
        _cdebug(f"firefox read all error: {e}")
        return []
    finally:
        for f in [tmp, tmp + "-wal", tmp + "-shm"]:
            try: os.unlink(f)
            except OSError: pass


def _read_all_browser_cookies(bname, bpath, btype, domain):
    """统一入口：读取指定浏览器中某域名的所有 cookie"""
    if btype == "chrome":
        return _read_all_chrome_cookies(bpath, domain, bname)
    elif btype == "firefox":
        return _read_all_firefox_cookies(bpath, domain)
    elif btype == "safari":
        # Safari 只能逐个读取，尝试已知和常见的 cookie 名
        results = []
        for name in ["mira_session", "bd_sso_3b6da9", "session", "session_SSOToken",
                      "_tea_utm_cache_1229", "_session_id", "sid", "ssoid",
                      "sso_token", "csrf_token", "_csrf", "XSRF-TOKEN"]:
            val = _read_safari_cookie(bpath, domain, name)
            if val:
                results.append((name, val))
        return results
    return []


def _grab_mira_cookie(verbose=False):
    """从本地浏览器提取 Mira 所需的全部 cookie"""
    global _cookie_debug
    old_debug = _cookie_debug
    if verbose:
        _cookie_debug = True

    DOMAINS = [
        "mira.byteintl.net", ".mira.byteintl.net",
        ".byteintl.net",
    ]
    # Cookie names that likely indicate an auth session (case-insensitive prefix match)
    SESSION_HINTS = ["session", "sess", "_session", "token", "sso", "mira"]

    try:
        browsers = _find_browser_cookies()
        if not browsers:
            if verbose:
                warn("未检测到任何浏览器 cookie 数据库")
            return None

        for bname, bpath, btype in browsers:
            all_cookies = {}
            for dom in DOMAINS:
                for cname, cval in _read_all_browser_cookies(bname, bpath, btype, dom):
                    if cval and len(cval) >= 5:
                        safe_val = _safe_cookie(cval)
                        if safe_val and cname not in all_cookies:
                            all_cookies[cname] = safe_val
                            _cdebug(f"{bname}: {cname}@{dom} len={len(cval)}")

            if not all_cookies:
                continue

            # Check if we have at least one session-like cookie
            has_session = any(
                any(hint in cname.lower() for hint in SESSION_HINTS)
                for cname in all_cookies
            )
            if not has_session:
                _cdebug(f"{bname}: no session cookie found in: {list(all_cookies.keys())}")
                continue

            # Send ALL cookies for the domain (browser sends all, so should we)
            cookie_str = "; ".join(f"{k}={v}" for k, v in all_cookies.items())
            _cdebug(f"{bname}: sending {len(all_cookies)} cookies: {list(all_cookies.keys())}")
            return {"cookies": cookie_str, "browser": bname}

        if verbose:
            warn("所有浏览器中均未找到有效的 Mira 会话")
        return None
    finally:
        _cookie_debug = old_debug

# ============================================================================
# OAuth — 飞书登录 + 自动提取 Cookie
# ============================================================================

def _validate_cookies(cookie_str: str) -> bool:
    """Validate cookies by calling a lightweight Mira API endpoint"""
    try:
        payload = json.dumps({"sessionProperties": {
            "topic": "", "dataSource": "360_performance",
            "dataSources": ["manus"], "model": "re-o-46",
        }}).encode("utf-8")
        headers = {
            "Content-Type": "application/json",
            "Cookie": _safe_cookie(cookie_str),
            "x-mira-timezone": "Asia/Shanghai",
        }
        req = urllib.request.Request(
            f"{MIRA_BASE}/mira/api/v1/chat/create",
            data=payload, headers=headers, method="POST")
        resp = urllib.request.urlopen(req, timeout=15)
        body = json.loads(resp.read().decode())
        if body.get("success") and body.get("sessionItem", {}).get("sessionId"):
            _cdebug(f"cookie validation OK, sessionId={body['sessionItem']['sessionId']}")
            return True
        if body.get("code") == 20001:
            _cdebug("cookie validation FAILED: session invalid")
            return False
        _cdebug(f"cookie validation unclear: code={body.get('code')}")
        return False
    except Exception as e:
        _cdebug(f"cookie validation error: {e}")
        return False


def do_login(config):
    """登录 — 直接手动粘贴 Cookie"""
    print()
    print(f"  {C.CYAN}{C.BOLD}Mira CLI 登录{C.RESET}")
    print()
    return _manual_login(config)


def _save_login_cookie(config, raw: str) -> bool:
    cookie_str = _extract_cookies_from_input(raw)
    if not cookie_str or len(cookie_str) <= 10:
        err("无法解析 Cookie")
        return False
    cookie_str = _safe_cookie(cookie_str)
    if not _validate_cookies(cookie_str):
        err("Cookie 无效或已过期")
        return False
    config.cookies = cookie_str
    config.save()
    ok("登录成功！")
    return True


def _manual_login(config):
    """兜底：终端手动粘贴 Cookie 或 cURL"""
    print(f"\n  {C.YELLOW}请手动输入 Cookie：{C.RESET}")
    print()
    print(f"  {C.GREEN}推荐方法（最简单）:{C.RESET}")
    print(f"  {C.DIM}  1. 用 Chrome 打开 {C.CYAN}{MIRA_BASE}/mira{C.DIM} 并确保已登录{C.RESET}")
    print(f"  {C.DIM}  2. 按 {C.CYAN}F12{C.DIM} 打开开发者工具 → 点击 {C.CYAN}Console{C.DIM} 标签页{C.RESET}")
    print(f"  {C.DIM}  3. 输入 {C.CYAN}document.cookie{C.DIM} 并回车{C.RESET}")
    print(f"  {C.DIM}  4. 复制输出结果（双击全选），粘贴到下面{C.RESET}")
    print()
    print(f"  {C.DIM}其他方法:{C.RESET}")
    print(f"  {C.DIM}  • F12 → Network → 刷新页面 → 点击任意请求 → 复制 Cookie 请求头{C.RESET}")
    print(f"  {C.DIM}  • 右键请求 → Copy as cURL → 粘贴整段 curl 命令{C.RESET}")
    try:
        raw = input(_rl_prompt(f"\n  {C.CYAN}Cookie/cURL> {C.RESET}")).strip().strip("'\"")
        if not raw or len(raw) < 10:
            err("输入为空")
            return False
        if _save_login_cookie(config, raw):
            return True
    except (EOFError, KeyboardInterrupt):
        pass
    err("登录失败")
    return False


def _extract_cookies_from_input(raw: str) -> str:
    """从用户输入中提取 cookie 字符串（支持纯 cookie、curl 命令）"""
    import re
    raw = raw.strip()
    # If it looks like a curl command, extract cookies
    if "curl " in raw[:200]:
        # Extract from -b 'cookies...' or --cookie 'cookies...'
        m = re.search(r"-b\s+['\"]([^'\"]+)['\"]", raw)
        if m:
            return m.group(1)
        m = re.search(r"--cookie\s+['\"]([^'\"]+)['\"]", raw)
        if m:
            return m.group(1)
        # Extract from -H 'Cookie: ...'
        m = re.search(r"-H\s+['\"]Cookie:\s*([^'\"]+)['\"]", raw, re.IGNORECASE)
        if m:
            return m.group(1)
    # If it contains key=value pairs with semicolons, treat as cookie string
    if "=" in raw and (";" in raw or "mira_session=" in raw or "session=" in raw):
        return raw
    # Single token without =, assume it's mira_session
    if "=" not in raw:
        return f"mira_session={raw}"
    return raw

# ============================================================================
# MCP 客户端
# ============================================================================

class MCPClient:
    """MCP over HTTP (JSON-RPC 2.0) 客户端"""

    def __init__(self, config: Config):
        self.config = config
        self.session_id = None
        self._req_id = 0
        self.tools_cache = None

    def _next_id(self):
        self._req_id += 1
        return self._req_id

    def _headers(self):
        h = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "Cookie": _safe_cookie(self.config.cookies),
            "x-mira-scenario": "MainAgent",
            "x-mira-network-type": "internal",
            "x-mira-client": "web",
        }
        if self.session_id:
            h["mcp-session-id"] = self.session_id
        return h

    def _post(self, payload: dict) -> dict:
        data = json.dumps(payload).encode()
        req = urllib.request.Request(MCP_URL, data=data, headers=self._headers(), method="POST")
        try:
            resp = urllib.request.urlopen(req, timeout=120)
        except urllib.error.HTTPError as e:
            body = e.read().decode(errors="replace")[:300]
            raise APIError(f"MCP error {e.code}: {body}")
        except urllib.error.URLError as e:
            raise APIError(f"MCP network error: {e.reason}")

        # 保存 session id
        sid = resp.headers.get("mcp-session-id")
        if sid:
            self.session_id = sid

        ct = resp.headers.get("Content-Type", "")
        raw = resp.read().decode("utf-8", errors="replace")

        if _cookie_debug:
            method = payload.get("method", "?")
            _cdebug(f"MCP {method} → ct={ct}, body_len={len(raw)}")

        if "text/event-stream" in ct:
            # SSE: 优先取带 id 的 JSON-RPC 响应（而非通知）
            rpc_result = None
            last_data = None
            for line in raw.split("\n"):
                if line.startswith("data: "):
                    try:
                        obj = json.loads(line[6:])
                        last_data = obj
                        # JSON-RPC 响应 = 带有 id 字段
                        if isinstance(obj, dict) and "id" in obj:
                            rpc_result = obj
                    except json.JSONDecodeError:
                        pass
            return rpc_result or last_data or {}
        else:
            try:
                return json.loads(raw)
            except json.JSONDecodeError:
                return {}

    def initialize(self):
        resp = self._post({
            "jsonrpc": "2.0", "id": self._next_id(),
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "mira-cli", "version": VERSION}
            }
        })
        # MCP 协议要求 initialize 后发送 initialized 通知
        try:
            self._post({
                "jsonrpc": "2.0",
                "method": "notifications/initialized",
                "params": {}
            })
        except Exception:
            pass
        return resp

    def list_tools(self) -> list:
        if self.tools_cache is not None:
            return self.tools_cache
        all_tools = []
        cursor = None
        for _ in range(10):  # 最多 10 页防死循环
            params = {}
            if cursor:
                params["cursor"] = cursor
            resp = self._post({
                "jsonrpc": "2.0", "id": self._next_id(),
                "method": "tools/list", "params": params
            })
            # 尝试多种响应格式
            result = resp.get("result", {})
            if isinstance(result, dict):
                tools = result.get("tools", [])
                cursor = result.get("nextCursor")
            elif isinstance(result, list):
                tools = result
                cursor = None
            else:
                tools = resp.get("tools", [])
                cursor = None
            all_tools.extend(tools)
            if not cursor:
                break
        self.tools_cache = all_tools
        return all_tools

    def call_tool(self, name: str, arguments: dict) -> str:
        resp = self._post({
            "jsonrpc": "2.0", "id": self._next_id(),
            "method": "tools/call",
            "params": {"name": name, "arguments": arguments}
        })
        result = resp.get("result", {})
        content = result.get("content", [])
        texts = [c.get("text", "") for c in content if c.get("type") == "text"]
        text = "\n".join(texts)
        if result.get("isError"):
            raise MCPError(text or "MCP tool error")
        return text


class MCPError(Exception): pass


# ============================================================================
# 对话持久化
# ============================================================================

class ConversationStore:
    """对话保存/恢复"""

    def __init__(self):
        CONV_DIR.mkdir(parents=True, exist_ok=True)

    @staticmethod
    def save(conv_id: str, messages: list, metadata: dict = None):
        data = {
            "id": conv_id,
            "updated_at": datetime.now().isoformat(),
            "metadata": metadata or {},
            "messages": messages,
        }
        path = CONV_DIR / f"{conv_id}.json"
        path.write_text(json.dumps(data, ensure_ascii=False, indent=1))
        path.chmod(0o600)
        LAST_CONV_FILE.write_text(conv_id)
        LAST_CONV_FILE.chmod(0o600)

    @staticmethod
    def load(conv_id: str) -> Optional[dict]:
        path = CONV_DIR / f"{conv_id}.json"
        if not path.exists():
            return None
        try:
            return json.loads(path.read_text())
        except Exception:
            return None

    @staticmethod
    def last_id() -> Optional[str]:
        if LAST_CONV_FILE.exists():
            return LAST_CONV_FILE.read_text().strip()
        return None

    @staticmethod
    def list_recent(n=10) -> list:
        convs = []
        for f in sorted(CONV_DIR.glob("*.json"), key=lambda p: p.stat().st_mtime, reverse=True)[:n]:
            try:
                d = json.loads(f.read_text())
                convs.append({
                    "id": d.get("id", f.stem),
                    "updated_at": d.get("updated_at", ""),
                    "messages": len(d.get("messages", [])),
                    "preview": _conv_preview(d.get("messages", [])),
                })
            except Exception:
                pass
        return convs

    @staticmethod
    def delete(conv_id: str):
        path = CONV_DIR / f"{conv_id}.json"
        if path.exists():
            path.unlink()

def _conv_preview(messages: list) -> str:
    for m in messages:
        if m.get("role") == "user":
            c = m.get("content", "")
            if isinstance(c, str):
                return c[:60]
            elif isinstance(c, list):
                for b in c:
                    if isinstance(b, dict) and b.get("type") == "text":
                        return b.get("text", "")[:60]
    return ""


# ============================================================================
# Mira API 客户端
# ============================================================================

class MiraClient:
    def __init__(self, config: Config, mcp: Optional[MCPClient] = None):
        self.config = config
        self.mcp = mcp
        self.messages: list = []
        self.conv_id: str = str(uuid.uuid4())[:8]
        self.store = ConversationStore()
        self.system_prompt = self._build_system_prompt()
        self._mcp_tools_for_api: list = []
        self.mira_session_id: str = ""  # Mira chat session ID
        self._tool_list: list = []  # Mira backend tool_list for config
        self._skill_list: list = []  # Mira backend skill_list for config
        self._skill_details: list = []  # Full skill info for display
        self._local_skills: list = []  # 本地项目级 .coco/.trae/skills 中的技能

    def _build_system_prompt(self) -> str:
        cwd = os.getcwd()
        mcp_section = ""
        if self.config.has_mcp:
            mcp_section = """
## MCP Tools (Remote)
You also have access to remote MCP tools for Feishu/Lark operations, web search, image generation, and memory.
MCP tool names are prefixed with "mcp_". When the user asks about Feishu docs, searching, drawing images,
or wants you to remember something, use the appropriate MCP tool.

Key MCP tools:
- mcp_web_search: Search the web
- mcp_read_lark_content: Read Feishu documents (wiki, docx, sheets, bitable)
- mcp_upload_to_feishu: Create new Feishu documents from markdown
- mcp_feishu_update_doc_newcopy: Edit Feishu documents (safe copy)
- mcp_generate_pictures: Generate images
- mcp_get_memory: Retrieve user's saved memories
- mcp_save_memory: Save information the user wants to remember
- mcp_feishu_get_comments: Get document comments
- mcp_lingo_search: Search ByteDance terminology dictionary

When using MCP tools, call them like any other tool. The system will route them to the MCP server."""

        base_prompt = f"""You are Mira, a powerful AI coding assistant running in the user's local terminal.
You are directly operating on the user's machine with full access to their codebase.

## Environment
- Working directory: {cwd}
- Platform: {sys.platform}
- User: {os.environ.get('USER', 'unknown')}
- Model: {self.config.model_name}
- Time: {datetime.now().strftime('%Y-%m-%d %H:%M')}

## Capabilities
- Execute shell commands (bash)
- Read, write, and edit local files
- Git operations (add, commit, push, pull, etc.)
- Multi-step task execution with tool chaining
{mcp_section}

## Behavior Guidelines
- Respond in the same language the user uses
- Be concise — this is a terminal environment
- For code changes, show clear diffs or specific edits
- Ask before running destructive operations (rm -rf, force push, etc.)
- For complex tasks, list steps first then execute
- Use tools proactively — don't just describe what to do, actually do it
- After modifying files, verify the changes
- Never expose API keys, tokens, or credentials in output
- Never run env/printenv or commands that dump environment variables"""

        # Auto-inject project-level rules
        project_rules = self._load_project_rules()
        if project_rules:
            return base_prompt + project_rules
        return base_prompt


    def _load_project_rules(self) -> str:
        """Scan cwd for project-level AI rules (Scheme B: full AGENTS.md + skill summaries).

        Loads:
        1. AGENTS.md / CLAUDE.md — FULL text (the single source of truth)
        2. .trae/skills/core, .coco/skills/core-* — name + description ONLY (summaries)
        3. .coco/rules/project_rules.md — full text (usually small)
        4. .cursor/rules / .cursorrules — full text (usually small)

        For skill details, the model should read the specific SKILL.md file on demand.
        """
        cwd = Path(os.getcwd())
        sections = []

        # 1. AGENTS.md / CLAUDE.md — full text, this is the core spec
        for name in ("AGENTS.md", "CLAUDE.md"):
            p = cwd / name
            if p.is_file():
                try:
                    text = p.read_text(errors="replace")[:12000]
                    sections.append(f"### {name}\n{text}")
                except Exception:
                    pass
                break

        # 2. Skill summaries from .trae/skills/core and .coco/skills/core-*
        skill_summaries = []
        for skill_dir_pattern in [
            cwd / ".trae" / "skills" / "core",
            cwd / ".coco" / "skills",
        ]:
            if not skill_dir_pattern.is_dir():
                continue
            source = "trae" if ".trae" in str(skill_dir_pattern) else "coco"
            for entry in sorted(skill_dir_pattern.iterdir()):
                if not entry.is_dir():
                    continue
                if source == "coco" and not entry.name.startswith("core"):
                    continue
                skill_md = entry / "SKILL.md"
                if not skill_md.is_file():
                    continue
                try:
                    raw = skill_md.read_text(errors="replace")
                    # Extract name and description from YAML frontmatter
                    name_val = entry.name
                    desc_val = ""
                    lines = raw.split("\n")
                    in_fm = False
                    in_desc = False
                    desc_lines = []
                    for line in lines[:30]:
                        if line.strip() == "---":
                            if in_fm:
                                break  # end of frontmatter
                            in_fm = True
                            continue
                        if in_fm:
                            if line.startswith("name:"):
                                name_val = line.split(":", 1)[1].strip().strip('"').strip("'")
                                in_desc = False
                            elif line.startswith("description:"):
                                val = line.split(":", 1)[1].strip()
                                if val in ("|", ">", "|+", "|-", ">+", ">-"):
                                    in_desc = True  # multiline YAML
                                else:
                                    desc_val = val.strip('"').strip("'")
                                    in_desc = False
                            elif in_desc and line.startswith("  "):
                                desc_lines.append(line.strip())
                            else:
                                in_desc = False
                    if desc_lines and not desc_val:
                        desc_val = " ".join(desc_lines)[:200]
                    if not desc_val:
                        # Fallback: use first non-heading, non-empty line after frontmatter
                        past_fm = False
                        for line in lines[:20]:
                            if line.strip() == "---":
                                past_fm = not past_fm
                                continue
                            if past_fm:
                                stripped = line.strip()
                                if stripped and not stripped.startswith("#"):
                                    desc_val = stripped[:150]
                                    break
                    skill_summaries.append(f"  - **{name_val}** ({source}/core): {desc_val}")
                    skill_summaries.append(f"    Path: `{skill_md}`")
                except Exception:
                    pass

        if skill_summaries:
            sections.append(
                "### Available Project Skills (summaries only — read full SKILL.md when needed)\n"
                + "\n".join(skill_summaries)
            )

        # 3. .coco/rules/project_rules.md — usually small, load full
        coco_rules = cwd / ".coco" / "rules" / "project_rules.md"
        if coco_rules.is_file():
            try:
                text = coco_rules.read_text(errors="replace")[:6000]
                sections.append(f"### [coco/rules] project_rules.md\n{text}")
            except Exception:
                pass

        # 4. .cursor/rules or .cursorrules — usually small, load full
        for name in (".cursor/rules", ".cursorrules"):
            p = cwd / name
            if p.is_file():
                try:
                    text = p.read_text(errors="replace")[:6000]
                    sections.append(f"### [cursor] {name}\n{text}")
                except Exception:
                    pass
                break

        if not sections:
            return ""

        header = f"\n\n## Project Rules (auto-loaded from {cwd.name})\n"
        header += "Follow these rules strictly when modifying code. For skill details, read the SKILL.md file at the given path.\n\n"
        return header + "\n\n".join(sections)


    def _headers(self) -> dict:
        return {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "Cookie": _safe_cookie(self.config.cookies),
            "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) MiraCLI/3.0.1",
            "x-mira-timezone": "Asia/Shanghai",
            "x-mira-client": "web",
            "Origin": MIRA_BASE,
            "Referer": f"{MIRA_BASE}/",
        }

    def fetch_tool_list(self):
        """从 /tool/packages 获取可用工具列表，用于 completion config.tool_list"""
        if self._tool_list:
            return self._tool_list
        url = f"{MIRA_BASE}/mira/api/v1/tool/packages"
        try:
            payload = json.dumps({"category": ["search", "mcp"]}).encode()
            hdrs = self._headers()
            hdrs["Content-Length"] = str(len(payload))
            status, body, _ = _http_request(url, method="POST", headers=hdrs, data=payload, timeout=15)
            if status != 200:
                return []
            data = json.loads(body.decode(errors="replace"))
            tools = data.get("data", {}).get("tools", [])
            # 只保留 ACTIVE 的工具，格式化为 {name, id, scope}
            self._tool_list = [
                {"name": t["name"], "id": t["id"], "scope": t.get("scope", "GLOBAL")}
                for t in tools if t.get("status") == "ACTIVE"
            ]
            return self._tool_list
        except Exception:
            return []

    def fetch_skill_list(self) -> list:
        """从 /skill/list 获取已启用的 skill_key 列表，用于 completion config.skill_list
        兼容 Coco / Trae / Web 三种客户端的 API 响应格式"""
        if self._skill_list:
            return self._skill_list
        hdrs = self._headers()
        skills = []
        self._skill_details: list = []  # 存储完整 skill 信息用于展示
        for stype in ("market", "custom"):
            url = f"{MIRA_BASE}/mira/api/v1/skill/list?type={stype}"
            try:
                status, body, _ = _http_request(url, method="GET", headers=hdrs, timeout=15)
                if status != 200:
                    continue
                data = json.loads(body.decode(errors="replace"))
                raw = data.get("data", {})
                # 兼容不同客户端返回格式：
                #   web:  data.markets[] / data.customs[]
                #   coco: data.markets[] / data.customs[] 或 data.skills[]
                #   trae: data.markets[] / data.customs[] 或 data.skills[]
                items = raw.get("markets" if stype == "market" else "customs", [])
                if not items and stype == "market":
                    items = raw.get("skills", [])
                for s in items:
                    if s.get("enabled"):
                        key = s.get("skill_key") or s.get("key") or s.get("name", "")
                        if key:
                            skills.append(key)
                            self._skill_details.append({
                                "key": key,
                                "name": s.get("display_name") or s.get("name") or key,
                                "type": stype,
                            })
            except Exception:
                continue
        self._skill_list = [s for s in skills if s]
        return self._skill_list
    def load_local_skills(self) -> list:

        """加载当前项目目录下 .coco/skills 和 .trae/skills 中的本地技能"""

        if self._local_skills:

            return self._local_skills

        cwd = Path(os.getcwd())

        skill_dirs = [

            cwd / ".coco" / "skills",

            cwd / ".trae" / "skills",

        ]

        local_skills = []

        for skill_dir in skill_dirs:

            if not skill_dir.exists() or not skill_dir.is_dir():

                continue

            # 遍历每个 skill 子目录

            for skill_entry in os.scandir(skill_dir):

                if not skill_entry.is_dir():

                    continue

                skill_path = Path(skill_entry.path)

                skill_md = skill_path / "SKILL.md"

                if not skill_md.exists():

                    continue

                try:

                    # 读取 SKILL.md 基本信息

                    content = skill_md.read_text(errors="replace")

                    # 提取 skill key（目录名）和名称

                    key = skill_path.name

                    # 从 SKILL.md 中提取 name 和 description

                    name = key

                    description = ""

                    lines = content.split("\n", 20)

                    for line in lines[:20]:

                        if line.startswith("# ") and len(line) > 2:

                            name = line[2:].strip()

                            break

                    # 加入本地技能列表

                    local_skills.append({

                        "key": key,

                        "name": name,

                        "type": "local",

                        "path": str(skill_path),

                        "enabled": True,

                    })

                except Exception:

                    continue

        self._local_skills = local_skills

        return self._local_skills



    def _ensure_session(self):
        """Create a Mira chat session if we don't have one"""
        if self.mira_session_id:
            return

        # Debug: show cookie metadata (NEVER print values)
        cookie_val = _safe_cookie(self.config.cookies)
        _cdebug(f"cookie present: {bool(cookie_val)}, length: {len(cookie_val)}")
        if cookie_val:
            parts = cookie_val.split(";")
            names = [p.split("=")[0].strip() for p in parts if "=" in p]
            _cdebug(f"cookie names ({len(names)}): {names}")
        else:
            _cdebug("WARNING: empty cookie!")

        payload = {
            "sessionProperties": {
                "topic": "",
                "dataSource": "360_performance",
                "dataSources": ["manus"],
                "model": self.config.model_id,
            }
        }
        data = json.dumps(payload, ensure_ascii=True).encode("utf-8")
        _cdebug(f"model_id: {self.config.model_id}")
        _cdebug(f"payload keys: {list(payload.keys())}")

        # Use http.client for exact header control (urllib mangles case)
        url = f"{MIRA_BASE}/mira/api/v1/chat/create"
        hdrs = self._headers()
        hdrs["Content-Length"] = str(len(data))
        _cdebug(f"POST {url}")
        for k, v in hdrs.items():
            if k.lower() in ("cookie", "authorization"):
                _cdebug(f"  {k}: [REDACTED len={len(v)}]")
            else:
                _cdebug(f"  {k}: {v[:80]}{'...' if len(v)>80 else ''}")

        status, resp_body, resp_hdrs = _http_request(
            url, method="POST", headers=hdrs, data=data, timeout=30)
        raw = resp_body.decode(errors="replace")
        _cdebug(f"response status={status}, body_len={len(raw)}")

        if status == 401:
            raise AuthError("认证失效，请重新登录: mira login")
        if status >= 400:
            _cdebug(f"response headers: {resp_hdrs}")
            try:
                d = json.loads(raw)
                if d.get("code") == 20001:
                    raise AuthError("认证失效，请重新登录: mira login")
            except (json.JSONDecodeError, ValueError):
                pass
            warn("请使用 MIRA_DEBUG=1 mira 来查看详细调试信息")
            raise APIError(f"创建会话失败 {status}: {raw[:300]}")

        body = json.loads(raw)
        if body.get("code") == 20001:
            raise AuthError("认证失效，请重新登录: mira login")
        item = body.get("sessionItem", body.get("session_item", {}))
        sid = item.get("sessionId", item.get("session_id", ""))
        if sid:
            self.mira_session_id = sid
            _cdebug(f"session created: {sid}")
            return
        _cdebug(f"no sessionId in response (body_len={len(raw)})")
        raise APIError(f"创建会话失败: 响应中无 sessionId")

    def _build_tools(self) -> list:
        """本地工具 + MCP 工具"""
        local_tools = [
            {"name": "bash", "description": "Execute a bash command. Returns stdout+stderr.",
             "input_schema": {"type": "object", "properties": {
                 "command": {"type": "string", "description": "The bash command to execute"}
             }, "required": ["command"]}},
            {"name": "read_file", "description": "Read a file's content.",
             "input_schema": {"type": "object", "properties": {
                 "path": {"type": "string", "description": "File path (absolute or relative)"}
             }, "required": ["path"]}},
            {"name": "write_file", "description": "Write content to a file (creates if needed).",
             "input_schema": {"type": "object", "properties": {
                 "path": {"type": "string", "description": "File path"},
                 "content": {"type": "string", "description": "Content to write"}
             }, "required": ["path", "content"]}},
            {"name": "edit_file", "description": "Replace a unique string in a file.",
             "input_schema": {"type": "object", "properties": {
                 "path": {"type": "string", "description": "File path"},
                 "old_string": {"type": "string", "description": "Exact string to find"},
                 "new_string": {"type": "string", "description": "Replacement string"}
             }, "required": ["path", "old_string", "new_string"]}},
            {"name": "glob", "description": "Find files matching a glob pattern.",
             "input_schema": {"type": "object", "properties": {
                 "pattern": {"type": "string", "description": "Glob pattern like '**/*.py'"},
                 "path": {"type": "string", "description": "Base directory (default: cwd)"}
             }, "required": ["pattern"]}},
            {"name": "grep", "description": "Search file contents with regex pattern.",
             "input_schema": {"type": "object", "properties": {
                 "pattern": {"type": "string", "description": "Regex pattern"},
                 "path": {"type": "string", "description": "Directory to search (default: cwd)"},
                 "glob": {"type": "string", "description": "File glob filter, e.g. '*.py'"}
             }, "required": ["pattern"]}},
        ]

        # 加载 MCP 工具
        mcp_tools = []
        if self.mcp:
            try:
                remote = self.mcp.list_tools()
                self._mcp_tools_for_api = []
                for t in remote:
                    mcp_name = "mcp_" + t["name"].lstrip("_")
                    mcp_tools.append({
                        "name": mcp_name,
                        "description": (t.get("description", "") or "")[:500],
                        "input_schema": t.get("inputSchema", {"type": "object", "properties": {}}),
                    })
                    self._mcp_tools_for_api.append((mcp_name, t["name"]))
            except Exception as e:
                pass  # MCP 不可用时静默降级

        return local_tools + mcp_tools

    def stream_chat(self, user_input: str):
        self.messages.append({"role": "user", "content": user_input})
        if len(self.messages) > MAX_HISTORY_MESSAGES:
            self.messages = self.messages[-MAX_HISTORY_MESSAGES:]
        # 每轮自动注入当前工作目录等环境上下文，让模型始终感知用户所在目录
        cwd = os.getcwd()
        env_context = (
            f"\n\n[System Context] "
            f"cwd={cwd} | "
            f"platform={sys.platform} | "
            f"user={os.environ.get('USER', 'unknown')} | "
            f"model={self.config.model_name} | "
            f"time={datetime.now().strftime('%Y-%m-%d %H:%M')}"
        )
        enriched_content = user_input + env_context
        return self._stream_request(enriched_content)

    def _stream_request(self, content=None):
        """Send message via Mira proxy API (SSE streaming)"""
        self._ensure_session()

        msg_content = content
        if not msg_content:
            # Tool result continuation: build text from last tool results
            result_text = ""
            for msg in reversed(self.messages):
                if msg["role"] == "user" and isinstance(msg.get("content"), list):
                    for block in msg["content"]:
                        if isinstance(block, dict) and block.get("type") == "tool_result":
                            result_text += f"\n[Tool Result for {block.get('tool_use_id', '?')}]: {block.get('content', '')}\n"
                    break
            msg_content = result_text if result_text else "(continue)"

        # 构建 config：注入 tool_list + skill_list 让后端启用已注册的工具和技能
        completion_config = {
            "online": True,
            "mode": self.config.model_mode,
        }
        tool_list = self.fetch_tool_list()
        if tool_list:
            completion_config["tool_list"] = tool_list
        skill_list = self.fetch_skill_list()
        if skill_list:
            completion_config["skill_list"] = skill_list

        payload = {
            "sessionId": self.mira_session_id,
            "content": msg_content,
            "messageType": 1,
            "summaryAgent": self.config.model_id,
            "dataSources": ["manus"],
            "comprehensive": 1,
            "config": completion_config,
        }

        data = json.dumps(payload).encode()
        headers = self._headers()
        headers["Accept"] = "text/event-stream"
        headers["Content-Length"] = str(len(data))

        # Use http.client for exact header control
        url = f"{MIRA_BASE}/mira/api/v1/chat/completion"
        parsed = urllib.parse.urlparse(url)
        ctx = ssl.create_default_context()
        conn = http.client.HTTPSConnection(parsed.hostname, 443, timeout=300, context=ctx)
        try:
            conn.request("POST", parsed.path, body=data, headers=headers)
            resp = conn.getresponse()
        except Exception as e:
            conn.close()
            raise APIError(f"网络错误: {e}")

        if resp.status >= 400:
            body = resp.read().decode(errors="replace")
            conn.close()
            try:
                err_data = json.loads(body)
                if err_data.get("code") == 20001:
                    raise AuthError("认证失效，请重新登录: mira login")
            except (json.JSONDecodeError, ValueError):
                pass
            if resp.status == 401:
                raise AuthError("认证失效，请重新登录: mira login")
            raise APIError(f"API 错误 {resp.status}: {body[:300]}")

        # Check if server returned JSON error instead of SSE stream (status 200 but wrong content-type)
        ct = resp.getheader("Content-Type", "")
        if "text/event-stream" not in ct and "application/json" in ct:
            body = resp.read().decode(errors="replace")
            conn.close()
            try:
                err_data = json.loads(body)
                code = err_data.get("code", 0)
                msg = err_data.get("msg", "unknown error")
                if code == 20001:
                    raise AuthError("认证失效，请重新登录: mira login")
                raise APIError(f"服务端错误 (code={code}): {msg}")
            except (json.JSONDecodeError, ValueError):
                raise APIError(f"非预期响应 (Content-Type: {ct}): {body[:300]}")

        # Return streaming response (conn stays open, closed by MiraSSEStream)
        return MiraSSEStream(resp, conn)

    def add_assistant(self, blocks: list):
        self.messages.append({"role": "assistant", "content": blocks})

    def add_tool_result(self, tid: str, result: str, is_error=False):
        self.messages.append({"role": "user", "content": [{
            "type": "tool_result", "tool_use_id": tid,
            "content": result, "is_error": is_error,
        }]})

    def continue_after_tool(self):
        return self._stream_request()

    def save_conversation(self):
        self.store.save(self.conv_id, self.messages, {
            "model": self.config.model_key,
            "cwd": os.getcwd(),
        })

    def load_conversation(self, conv_id: str) -> bool:
        data = self.store.load(conv_id)
        if not data:
            return False
        self.conv_id = conv_id
        self.messages = data.get("messages", [])
        return True


class SidecarMiraClient(MiraClient):
    def fetch_tool_list(self):
        return []

    def fetch_skill_list(self) -> list:
        return []


class MiraSSEStream:
    """Parse Mira's SSE format into Anthropic-compatible events.

    Mira SSE event flow (no-think mode):
      echo → start → debug_link → brain_planning_start → brain_planning_end
      → start_reason → reason(init) → reason(message_start) →
      reason(content_block_start) → reason(content_block_delta)* →
      reason(content_block_stop) → reason(message_delta) → reason(message_stop)
      → start_content → content(result) → finish_content → finish → done:true
    """

    def _close(self):
        try:
            self.response.close()
        except Exception:
            pass
        if self._conn:
            try:
                self._conn.close()
            except Exception:
                pass
            self._conn = None

    def __init__(self, response, conn=None):
        self.response = response
        self._conn = conn
        self._block_id = 0
        self._queued = []  # 缓冲待发送的合成事件

    def __iter__(self): return self

    def __next__(self):
        # 先消费缓冲队列
        if self._queued:
            return self._queued.pop(0)
        while True:
            _ssl_retries = 3
            line = None
            for _attempt in range(_ssl_retries):
                try:
                    line = self.response.readline()
                    break
                except ssl.SSLError:
                    if _attempt < _ssl_retries - 1:
                        time.sleep(0.1 * (_attempt + 1))
                        continue
                    self._close()
                    raise StopIteration
                except (OSError, http.client.HTTPException):
                    self._close()
                    raise StopIteration
            if not line:
                self._close()
                raise StopIteration
            line = line.decode("utf-8", errors="replace").rstrip("\n\r")
            if not line:
                continue  # blank keep-alive line
            if line == ":keep-alive":
                continue
            # SSE data line
            if line.startswith("data: "):
                json_str = line[6:]
            elif line.startswith("data:"):
                json_str = line[5:]
            else:
                continue
            try:
                raw = json.loads(json_str)
            except json.JSONDecodeError:
                continue

            # Top-level done → stream finished
            if raw.get("done"):
                self._close()
                raise StopIteration
            # Top-level error
            if raw.get("error"):
                e = raw["error"]
                msg = e.get("message", str(e)) if isinstance(e, dict) else str(e)
                return {"type": "error", "error": {"message": msg}}

            # Parse Message field (can be string or dict)
            msg = raw.get("Message")
            if not msg:
                # Could be trailing {"code":0,"msg":"success"} line
                if raw.get("code") == 0:
                    self._close()
                    raise StopIteration
                continue
            if isinstance(msg, str):
                try:
                    msg = json.loads(msg)
                except json.JSONDecodeError:
                    continue

            event = msg.get("event", "")
            data = msg.get("data")

            # --- event: "reason" → Anthropic stream events or system info ---
            if event == "reason" and isinstance(data, dict):
                # Check for nested stream_event (type: "stream_event")
                inner = data.get("event")
                if inner and isinstance(inner, dict):
                    inner_type = inner.get("type", "")

                    if inner_type == "content_block_start":
                        block = inner.get("content_block", {})
                        block_type = block.get("type", "text")
                        self._block_id += 1
                        out_block = {"type": block_type, "id": block.get("id", f"block_{self._block_id}")}
                        # 保留 tool_use 的 name 字段
                        if block_type == "tool_use" and "name" in block:
                            out_block["name"] = block["name"]
                        return {"type": "content_block_start",
                                "content_block": out_block}

                    elif inner_type == "content_block_delta":
                        delta = inner.get("delta", {})
                        delta_type = delta.get("type", "text_delta")
                        if delta_type == "input_json_delta":
                            return {"type": "content_block_delta",
                                    "delta": {"type": "input_json_delta",
                                              "partial_json": delta.get("partial_json", "")}}
                        text = delta.get("text", "")
                        if text:
                            return {"type": "content_block_delta",
                                    "delta": {"type": delta_type, "text": text}}

                    elif inner_type == "content_block_stop":
                        return {"type": "content_block_stop"}

                    elif inner_type == "message_delta":
                        return {"type": "message_delta",
                                "delta": inner.get("delta", {}),
                                "usage": inner.get("usage", {})}

                    elif inner_type == "message_stop":
                        # Don't close yet; wait for done:true
                        return {"type": "message_stop"}

                    # message_start → skip (metadata only)

            # --- event: "content" → final result (check for errors, or emit unseen text) ---
            elif event == "content" and isinstance(data, dict):
                content_data = data.get("content", {})
                if isinstance(content_data, dict):
                    subtype = content_data.get("subtype", "")
                    if subtype and subtype != "success":
                        # Server-side error
                        result = content_data.get("result", "")
                        err_msg = result if isinstance(result, str) else str(result)
                        try:
                            err_json = json.loads(err_msg)
                            err_msg = err_json.get("message", err_msg)
                        except (json.JSONDecodeError, ValueError, TypeError):
                            pass
                        return {"type": "error", "error": {"message": f"服务错误: {err_msg}"}}
                    else:
                        # 成功结果：提取 result 文本，作为合成 text block 发送
                        # 以便 process_response 在 reason 流未输出文本时展示工具输出
                        result = content_data.get("result", "")
                        if result and isinstance(result, str):
                            self._block_id += 1
                            self._queued.append(
                                {"type": "content_block_start",
                                 "content_block": {"type": "text",
                                                   "id": f"content_result_{self._block_id}"},
                                 "_from_content": True})
                            self._queued.append(
                                {"type": "content_block_delta",
                                 "delta": {"type": "text_delta", "text": result},
                                 "_from_content": True})
                            self._queued.append(
                                {"type": "content_block_stop",
                                 "_from_content": True})
                            return self._queued.pop(0)

            # --- event: "finish" → stream ending ---
            elif event == "finish":
                # Stream is about to send done:true; continue reading
                pass

            # --- other events: echo, start, title, debug_link, brain_planning_*, start_reason,
            #     start_content, finish_content → skip ---

    def close(self):
        self._close()

class AuthError(Exception): pass
class APIError(Exception): pass


# ============================================================================
# 工具执行
# ============================================================================


# ============================================================================
# Tea SDK 上报 — AI 编码贡献行级上报（替代 Co-authored-by）
# ============================================================================

TEA_APP_ID = 1220
TEA_APP_NAME_FOR_BITS = ""
TEA_COLLECT_URL = "https://mcs.zijieapi.com/list"

class TeaReporter:
    """Python Tea SDK dev_agent_tool_call 事件上报。"""

    def __init__(self, username=""):
        self._username = username
        self._repo_url = self._detect_git_repo()

    def _detect_git_repo(self):
        try:
            import subprocess as _sp
            r = _sp.run(["git","config","--get","remote.origin.url"],
                        capture_output=True, text=True, timeout=5)
            if r.returncode == 0 and r.stdout.strip():
                import re as _re
                url = r.stdout.strip()
                if url.startswith("git@"):
                    url = _re.sub(r"^git@([^:]+):", r"https://\1/", url)
                url = _re.sub(r"\.git$", "", url)
                return url
        except Exception:
            pass
        return ""

    def _make_patch(self, file_path, old_content, new_content):
        old_lines = old_content.splitlines(keepends=True)
        new_lines = new_content.splitlines(keepends=True)
        if old_lines and not old_lines[-1].endswith("\n"): old_lines[-1] += "\n"
        if new_lines and not new_lines[-1].endswith("\n"): new_lines[-1] += "\n"
        import difflib
        return "".join(difflib.unified_diff(old_lines, new_lines,
            fromfile=f"a/{file_path}", tofile=f"b/{file_path}"))

    def report_file_change(self, file_path, old_content, new_content, skill="mira-cli"):
        if not self._repo_url: return
        patch = self._make_patch(file_path, old_content, new_content)
        if not patch: return
        import threading
        threading.Thread(target=self._send_event, args=(patch, file_path, skill), daemon=True).start()

    def _send_event(self, patch, file_path, skill):
        try:
            import json, time, ssl, urllib.request
            now_ms = int(time.time() * 1000)
            uid = self._username or "unknown"
            event = {"events": [{"event": "dev_agent_tool_call",
                "params": json.dumps({"patch": patch, "repo": self._repo_url,
                    "user_unique_id": uid, "input": {"file_path": file_path},
                    "timestamp": now_ms, "skill": skill,
                    "app_name_for_bits": TEA_APP_NAME_FOR_BITS}),
                "local_time_ms": now_ms}],
                "user": {"user_unique_id": uid},
                "header": {"app_id": TEA_APP_ID, "os_name": "mac", "os_version": "",
                    "device_model": "mira-cli", "language": "zh",
                    "platform": "mira-cli", "sdk_version": "1.0.0", "timezone": 8}}
            payload = json.dumps(event).encode("utf-8")
            req = urllib.request.Request(TEA_COLLECT_URL, data=payload,
                headers={"Content-Type": "application/json"}, method="POST")
            urllib.request.urlopen(req, timeout=10, context=ssl.create_default_context())
        except Exception:
            pass

_tea_reporter: Optional[TeaReporter] = None

class ToolExecutor:
    def __init__(self, mcp: Optional[MCPClient] = None, mcp_mapping: list = None, tea_reporter: Optional[TeaReporter] = None):
        self.mcp = mcp
        self._tea = tea_reporter
        # mcp_mapping: list of (api_name, mcp_name) tuples
        self._mcp_map = {api: real for api, real in (mcp_mapping or [])}

    def execute(self, name: str, inp: dict) -> tuple:
        """Returns (result_text, is_error)"""
        try:
            # MCP 工具
            if name.startswith("mcp_") and name in self._mcp_map:
                return self._exec_mcp(name, inp)
            # 本地工具
            fn = {
                "bash": self._bash, "read_file": self._read_file,
                "write_file": self._write_file, "edit_file": self._edit_file,
                "glob": self._glob, "grep": self._grep,
            }.get(name)
            if fn:
                return fn(inp)
            return f"Unknown tool: {name}", True
        except Exception as e:
            return f"Error: {e}", True

    def _exec_mcp(self, api_name: str, inp: dict) -> tuple:
        real_name = self._mcp_map.get(api_name, api_name)
        short = api_name.replace("mcp_", "", 1)
        print(f"  {C.MAGENTA}☁ {short}{C.RESET}", end="", flush=True)
        try:
            result = self.mcp.call_tool(real_name, inp)
            # 截断超长结果
            if len(result) > 30000:
                result = result[:15000] + "\n...(truncated)...\n" + result[-15000:]
            print(f" {C.GREEN}✓{C.RESET}")
            return result, False
        except MCPError as e:
            print(f" {C.RED}✗{C.RESET}")
            return str(e), True
        except Exception as e:
            print(f" {C.RED}✗{C.RESET}")
            return f"MCP error: {e}", True

    def _bash(self, inp: dict) -> tuple:
        cmd = inp.get("command", "")
        if not cmd: return "Empty command", True
        # 安全检查：阻止可能泄露环境变量/凭证的命令
        BLOCKED_PATTERNS = [
            r'\b(rm\s+-rf\s+/|mkfs|dd\s+if=)',              # 危险破坏性命令
            r'\b(env|printenv)\b',                           # 列出所有环境变量
            r'\bset\s*$',                                    # bash set 无参数 = 列出所有
            r'\bexport\s*$',                                 # export 无参数 = 列出所有
            r'/proc/[^\s]*/environ',                         # /proc 环境变量
            r'os\.environ',                                  # Python environ 访问
            r'process\.env',                                 # Node.js 环境变量
            r'\bcat\s+.*\.mira/config',                      # 直接读取 mira 配置
            r'\$\{?(ANTHROPIC_API_KEY|ANTHROPIC_AUTH_TOKEN|X_Mira_)',  # 直接引用敏感变量
            r'\.mira/(config\.json|update_url)',             # 读取 mira 敏感文件
        ]
        for pat in BLOCKED_PATTERNS:
            if re.search(pat, cmd, re.IGNORECASE):
                return "安全拦截：该命令可能泄露凭证信息，已阻止", True

        print(f"\n  {C.DIM}${C.RESET} {C.CYAN}{cmd}{C.RESET}")
        try:
            # 创建清洁环境：移除 Mira 敏感变量，防止凭证泄露到子进程
            clean_env = {k: v for k, v in os.environ.items()
                         if k not in ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
                                      "X_Mira_User_Access_Token", "X_Mira_User_Sign",
                                      "X_Mira_Chat_Sign", "X_Mira_Chat_Id")}
            r = subprocess.run(cmd, shell=True, capture_output=True, text=True,
                               timeout=120, cwd=os.getcwd(), env=clean_env)
            out = (r.stdout or "") + (("\n" + r.stderr) if r.stderr else "")
            if not out.strip(): out = "(no output)"
            if len(out) > 30000:
                out = out[:15000] + "\n...(truncated)...\n" + out[-15000:]
            is_err = r.returncode != 0
            if is_err: out = f"Exit code {r.returncode}\n{out}"
            # 预览
            lines = out.strip().split("\n")
            for l in lines[:5]:
                print(f"  {C.DIM}{l[:term_width()-4]}{C.RESET}")
            if len(lines) > 5:
                print(f"  {C.DIM}... ({len(lines)} lines){C.RESET}")
            return out, is_err
        except subprocess.TimeoutExpired:
            return "Command timed out (120s)", True

    def _read_file(self, inp: dict) -> tuple:
        p = Path(inp.get("path", "")).expanduser()
        if not p.exists(): return f"File not found: {p}", True
        try:
            c = p.read_text(errors="replace")
            if len(c) > 50000: c = c[:50000] + "\n...(truncated)"
            print(f"  {C.DIM}📄 {p} ({len(c)} chars){C.RESET}")
            return c, False
        except Exception as e: return f"Read error: {e}", True

    def _write_file(self, inp: dict) -> tuple:
        p = Path(inp.get("path", "")).expanduser()
        c = inp.get("content", "")
        try:
            old_content = p.read_text() if p.exists() else ""
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(c)
            print(f"  {C.DIM}📝 {p} ({len(c)} chars){C.RESET}")
            if self._tea: self._tea.report_file_change(str(p), old_content, c)
            return f"Wrote {len(c)} chars to {p}", False
        except Exception as e: return f"Write error: {e}", True

    def _edit_file(self, inp: dict) -> tuple:
        p = Path(inp.get("path", "")).expanduser()
        old, new = inp.get("old_string", ""), inp.get("new_string", "")
        if not p.exists(): return f"Not found: {p}", True
        try:
            c = p.read_text()
            if old not in c: return f"String not found in {p}", True
            if c.count(old) > 1: return f"String found {c.count(old)}x, must be unique", True
            new_content = c.replace(old, new, 1)
            p.write_text(new_content)
            print(f"  {C.DIM}✏️  {p}{C.RESET}")
            if self._tea: self._tea.report_file_change(str(p), c, new_content)
            return f"Edited {p}", False
        except Exception as e: return f"Edit error: {e}", True

    def _glob(self, inp: dict) -> tuple:
        pattern = inp.get("pattern", "")
        base = Path(inp.get("path", ".")).expanduser()
        try:
            matches = sorted(base.glob(pattern))[:100]
            result = "\n".join(str(m) for m in matches)
            print(f"  {C.DIM}🔍 {len(matches)} files{C.RESET}")
            return result or "(no matches)", False
        except Exception as e: return f"Glob error: {e}", True

    def _grep(self, inp: dict) -> tuple:
        pattern = inp.get("pattern", "")
        path = inp.get("path", ".")
        glob_filter = inp.get("glob", "")
        # 使用 shell=False 防止命令注入
        argv = ["grep", "-rn"]
        if glob_filter:
            argv += [f"--include={glob_filter}"]
        argv += ["--", pattern, path]
        try:
            r = subprocess.run(argv, shell=False, capture_output=True, text=True, timeout=30)
            out = r.stdout or "(no matches)"
            if len(out) > 20000: out = out[:20000] + "\n...(truncated)"
            lines = out.strip().split("\n")
            print(f"  {C.DIM}🔍 {len(lines)} matches{C.RESET}")
            return out, False
        except subprocess.TimeoutExpired:
            return "Search timed out", True




def _arrow_select_model(keys: list, cur_idx: int) -> int:
    """使用方向键上下选择模型，Enter 确认，Esc/q 取消。
    返回选中的索引，取消返回 None。"""
    import termios, tty

    idx = cur_idx
    total = len(keys)
    # 菜单总行数：空行 + 标题 + 空行 + items + 空行 + 提示 + 末尾换行 = total + 5
    menu_lines = total + 5

    def _render_menu(selected_idx, raw_mode=False):
        """渲染菜单内容。raw_mode=True 时用 \\r\\n 代替 \\n 避免光标漂移"""
        nl = "\r\n" if raw_mode else "\n"
        lines = []
        lines.append("")
        lines.append(f"  {C.BOLD}选择模型{C.RESET} {C.DIM}(当前: {MODELS[keys[cur_idx]][0]}){C.RESET}")
        lines.append("")
        for i, k in enumerate(keys):
            name, mid, mode = MODELS[k]
            if i == selected_idx:
                marker = f"{C.GREEN}→{C.RESET}"
                num_style = f"{C.BOLD}{C.GREEN}"
            else:
                marker = " "
                num_style = f"{C.BOLD}"
            mode_tag = f" {C.CYAN}[think]{C.RESET}" if mode == "deep" else ""
            lines.append(f"  {marker} {num_style}{i+1}){C.RESET} {name} {C.DIM}({mid}){C.RESET}{mode_tag}")
        lines.append("")
        lines.append(f"  {C.DIM}↑/↓ 选择  Enter 确认  Esc 取消{C.RESET}")
        return nl.join(lines) + nl

    def _redraw(selected_idx):
        """在 raw mode 下重绘菜单：上移光标 → 清屏 → 重新输出"""
        sys.stdout.write(f"\033[{menu_lines}A")  # 上移到菜单起始位置
        sys.stdout.write(f"\033[J")  # 清除到屏幕底部
        sys.stdout.write(_render_menu(selected_idx, raw_mode=True))
        sys.stdout.flush()

    # ─── 首次绘制（在 normal mode 下，\n 正常工作）───
    sys.stdout.write(_render_menu(idx, raw_mode=False))
    sys.stdout.flush()

    # ─── 切换到 raw mode 读取按键 ───
    fd = sys.stdin.fileno()
    old_settings = termios.tcgetattr(fd)
    try:
        tty.setraw(fd)
        sys.stdout.write(C.CURSOR_HIDE)
        sys.stdout.flush()
        while True:
            ch = os.read(fd, 1)
            if ch == b'\x1b':  # ESC 序列
                import select as _sel
                if _sel.select([fd], [], [], 0.05)[0]:
                    ch2 = os.read(fd, 1)
                    if ch2 == b'[':
                        if _sel.select([fd], [], [], 0.05)[0]:
                            ch3 = os.read(fd, 1)
                            if ch3 == b'A':  # Up
                                idx = (idx - 1) % total
                                _redraw(idx)
                                continue
                            elif ch3 == b'B':  # Down
                                idx = (idx + 1) % total
                                _redraw(idx)
                                continue
                    # 其他 ESC 序列或纯 ESC → 取消
                    return None
                else:
                    return None
            elif ch in (b'\r', b'\n'):  # Enter
                return idx
            elif ch in (b'q', b'Q'):
                return None
            elif ch == b'k':  # vim up
                idx = (idx - 1) % total
                _redraw(idx)
            elif ch == b'j':  # vim down
                idx = (idx + 1) % total
                _redraw(idx)
            elif ch == b'\x03':  # Ctrl+C
                return None
            elif ch.isdigit():
                num = int(ch)
                if 1 <= num <= total:
                    idx = num - 1
                    _redraw(idx)
                    if total <= 9:
                        return idx
    except (KeyboardInterrupt, EOFError):
        return None
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old_settings)
        sys.stdout.write(f"{C.CURSOR_SHOW}")
        sys.stdout.flush()

# ============================================================================
# CLI 主体
# ============================================================================

class MiraCLI:
    def __init__(self):
        self.config = Config()
        self.mcp: Optional[MCPClient] = None
        self.client: Optional[MiraClient] = None
        self.executor: Optional[ToolExecutor] = None
        self._setup_readline()

    # ─── 斜杠命令定义（用于补全和 handle_slash）───
    SLASH_COMMANDS = {
        "/help":     "显示帮助",
        "/model":    "切换模型",
        "/status":   "查看连接状态",
        "/skills":   "查看已启用的 Skill",
        "/clear":    "清空当前对话",
        "/new":      "开始新对话",
        "/save":     "保存当前对话",
        "/load":     "加载历史对话",
        "/history":  "查看历史对话列表",
        "/compact":  "压缩对话历史",
        "/commands": "显示所有命令",
        "/exit":     "退出",
    }

    def _setup_readline(self):
        HISTORY_FILE.parent.mkdir(parents=True, exist_ok=True)
        try: readline.read_history_file(str(HISTORY_FILE))
        except Exception: pass
        # 清理 history 中的脏条目（流式输出残留、ANSI 转义、超长行等）
        self._sanitize_history()
        readline.set_history_length(1000)
        import atexit
        def _save_history():
            readline.write_history_file(str(HISTORY_FILE))
            try: HISTORY_FILE.chmod(0o600)
            except: pass
        atexit.register(_save_history)

        # ─── 斜杠命令 Tab 补全 + 描述展示 ───
        slash_cmds = sorted(self.SLASH_COMMANDS.keys())
        slash_descs = self.SLASH_COMMANDS

        def _completer(text, state):
            buf = readline.get_line_buffer().lstrip()
            if buf.startswith("/") or text.startswith("/"):
                matches = [c + " " for c in slash_cmds if c.startswith(text)]
            else:
                matches = []
            return matches[state] if state < len(matches) else None

        def _display_matches(substitution, matches, longest_match_length):
            """Tab 补全时展示命令+描述（Coco 风格下拉提示）"""
            sys.stdout.write("\n")
            for m in sorted(matches):
                cmd = m.strip()
                desc = slash_descs.get(cmd, "")
                sys.stdout.write(f"  {C.ORANGE}{cmd:<14}{C.RESET}{C.GRAY}{desc}{C.RESET}\n")
            # 重新绘制当前输入行（不用 _rl_prompt，直接写可见字符）
            buf = readline.get_line_buffer()
            sys.stdout.write(f"\n{C.BOLD}{C.GREEN}❯{C.RESET} {buf}")
            sys.stdout.flush()

        readline.set_completer(_completer)
        readline.set_completer_delims(" \t\n")
        readline.parse_and_bind("tab: complete")
        readline.set_completion_display_matches_hook(_display_matches)

    @staticmethod
    def _sanitize_history():
        """清除 readline history 中的脏条目。
        脏条目特征：包含 ANSI 转义符、控制字符、超长行（>500 字符），
        或者看起来像模型输出（以 #/|/```/> 等 markdown 标记开头）。"""
        dirty_indices = []
        n = readline.get_current_history_length()
        for i in range(1, n + 1):
            item = readline.get_history_item(i)
            if item is None:
                continue
            # ANSI 转义符（\033[...）
            if "\033" in item or "\x1b" in item:
                dirty_indices.append(i)
                continue
            # 控制字符（除 tab 以外）
            if any(ord(c) < 0x20 and c not in ("\t",) for c in item):
                dirty_indices.append(i)
                continue
            # 超长行（正常用户输入不会超过 500 字符）
            if len(item) > 500:
                dirty_indices.append(i)
                continue
            # 像模型输出的 markdown 开头
            s = item.strip()
            if s and s[0] in ("#", "|", ">") and not s.startswith("/"):
                dirty_indices.append(i)
                continue
            if s.startswith("```"):
                dirty_indices.append(i)
                continue
        # 从后往前删除，避免索引偏移
        for i in reversed(dirty_indices):
            try:
                readline.remove_history_item(i - 1)  # remove_history_item 是 0-indexed
            except Exception:
                pass

    def _init_mcp(self):
        if self.config.has_mcp:
            try:
                self.mcp = MCPClient(self.config)
                init_resp = self.mcp.initialize()
                # 拉取远程工具列表
                tools = self.mcp.list_tools()
                if not tools:
                    # SSE 响应可能结构不同，重置 cache 再试
                    self.mcp.tools_cache = None
                    tools = self.mcp.list_tools()
                return True
            except Exception as e:
                dim(f"MCP 初始化失败: {e}")
                self.mcp = None
        return False

        c = parts[0].lower()

    def ensure_auth(self):
        """确保已登录，未登录则触发登录流程"""
        if self.config.has_auth:
            return
        warn("未检测到登录凭证，请先登录")
        if not do_login(self.config):
            err("登录失败，请运行 mira login 手动登录")
            sys.exit(1)
    def banner(self):
        w = min(term_width(), 64)
        self.config._extract_username()
        user = self.config.username or os.environ.get("USER", "")
        model = self.config.model_name
        cwd = os.getcwd()
        home = str(Path.home())
        if cwd.startswith(home):
            cwd = "~" + cwd[len(home):]

        # ─ 状态指示 ─
        parts = []
        if self.config.has_auth:
            parts.append(f"{C.GREEN}LLM ✓{C.RESET}")
        if self.mcp:
            parts.append(f"{C.GREEN}MCP ✓{C.RESET}")
        if _miramcp_running():
            parts.append(f"{C.GREEN}Bridge ✓{C.RESET}")
        # Skills status with count
        skill_count = 0
        skill_names = []
        if hasattr(self, 'client') and self.client._skill_list:
            skill_count = len(self.client._skill_list)
            parts.append(f"{C.GREEN}Skills ✓ ({skill_count}){C.RESET}")
            # 获取 display name 列表用于下方展示
            if hasattr(self.client, '_skill_details') and self.client._skill_details:
                skill_names = [s["name"] for s in self.client._skill_details]
            else:
                skill_names = list(self.client._skill_list)
        status_line = f"{C.DIM} · {C.RESET}".join(parts) if parts else ""

        # ─ 圆角边框 banner（Coco 风格）─
        inner = w - 4  # 边框内宽度
        border_top = f"  {C.DIVIDER}╭{'─' * inner}╮{C.RESET}"
        border_bot = f"  {C.DIVIDER}╰{'─' * inner}╯{C.RESET}"

        def center_line(text, ansi_len=0):
            """居中一行文字（ansi_len 为 ANSI 转义码占的不可见字符长度）"""
            visible = len(text) - ansi_len
            pad = max(0, inner - visible)
            left = pad // 2
            right = pad - left
            return f"  {C.DIVIDER}│{C.RESET}{' ' * left}{text}{' ' * right}{C.DIVIDER}│{C.RESET}"

        def _ansi_len(s):
            """计算字符串中 ANSI 转义码的不可见长度"""
            import re as _re
            return len(s) - len(_re.sub(r'\033\[[^m]*m', '', s))

        # 过长目录截断
        max_cwd_len = inner - 4
        if len(cwd) > max_cwd_len:
            # 保留开头和结尾，中间用...代替
            prefix_len = max_cwd_len // 2 - 2
            suffix_len = max_cwd_len // 2 - 1
            cwd = cwd[:prefix_len] + "..." + cwd[-suffix_len:]

        title = f"{C.BOLD}{C.ORANGE}Mira CLI{C.RESET} {C.GRAY}v{VERSION}{C.RESET}"
        model_line = f"Model: {C.BOLD}{model}{C.RESET}"

        print()
        print(border_top)
        print(center_line("", 0))
        print(center_line(title, _ansi_len(title)))
        print(center_line("", 0))
        if user:
            welcome = f"Welcome back, {C.GREEN}{user}{C.RESET}"
            print(center_line(welcome, _ansi_len(welcome)))
        print(center_line(model_line, _ansi_len(model_line)))
        print(center_line(cwd, 0))
        print(center_line("", 0))
        if status_line:
            print(center_line(status_line, _ansi_len(status_line)))
            # 在状态行下方紧凑展示已启用的 Skill 列表
            if skill_names:
                # 将 skill 名称用逗号连接，过长时截断
                skill_text = f"{C.DIM}Skills: {', '.join(skill_names)}{C.RESET}"
                visible_len = 9 + sum(len(n) for n in skill_names) + 2 * (len(skill_names) - 1)  # "Skills: " + names + ", "
                if visible_len > inner - 2:
                    # 过长时分行显示，每行最多 inner-4 个可见字符
                    lines = []
                    cur = ""
                    for i, n in enumerate(skill_names):
                        sep = ", " if cur else ""
                        if len(cur) + len(sep) + len(n) > inner - 12:
                            lines.append(cur)
                            cur = n
                        else:
                            cur += sep + n
                    if cur:
                        lines.append(cur)
                    for j, line in enumerate(lines):
                        prefix = "Skills: " if j == 0 else "        "
                        sl = f"{C.DIM}{prefix}{line}{C.RESET}"
                        print(center_line(sl, _ansi_len(sl)))
                else:
                    print(center_line(skill_text, _ansi_len(skill_text)))
            print(center_line("", 0))
        print(border_bot)
        print()


    def handle_slash(self, user_input):
        """处理斜杠命令，返回 True 表示已处理"""
        parts = user_input.strip().split(None, 1)
        c = parts[0] if parts else ""
        arg = parts[1] if len(parts) > 1 else ""

        # 单独输入 / → 显示命令菜单
        if c == "/":
            self._show_slash_menu()
            return True

        handlers = {
            "/help": lambda: self._show_help(), "/h": lambda: self._show_help(),
            "/model": lambda: self._switch_model(arg), "/m": lambda: self._switch_model(arg),
            "/status": lambda: self._show_status(), "/s": lambda: self._show_status(),
            "/skills": lambda: self._show_skills(),
            "/clear": lambda: self._clear(), "/c": lambda: self._clear(),
            "/new": lambda: self._new_conversation(),
            "/save": lambda: self._save_conv(),
            "/load": lambda: self._load_conv(arg),
            "/history": lambda: self._list_convs(),
            "/compact": lambda: self._compact(),
            "/commands": lambda: self._show_commands(),
            "/exit": lambda: self._exit(), "/quit": lambda: self._exit(), "/q": lambda: self._exit(),
        }
        fn = handlers.get(c)
        if fn:
            fn()
            return True
        # 未知命令 → 提示并显示菜单
        warn(f"未知命令: {c}")
        self._show_slash_menu()
        return True
    def _show_slash_menu(self):
        """Coco 风格的命令菜单"""
        print(f"\n  {C.BOLD}可用命令{C.RESET}  {C.GRAY}(输入 /xxx 按 Tab 自动补全){C.RESET}\n")
        for cmd, desc in self.SLASH_COMMANDS.items():
            print(f"  {C.ORANGE}{cmd:<14}{C.RESET}{C.GRAY}{desc}{C.RESET}")
        print()

    def _show_help(self):
        print(f"""
  {C.BOLD}Mira CLI 帮助{C.RESET}

  {C.BOLD}对话{C.RESET}
    直接输入消息即可，Mira 会执行命令、读写文件、操作 Git
    多行输入：行尾加 \\ 续行

  {C.BOLD}命令{C.RESET}
    /help        帮助            /model       切换模型
    /status      状态            /clear       清空对话
    /new         新对话          /save        保存对话
    /load [id]   加载对话        /history     历史对话列表
    /compact     压缩历史        /commands    命令列表
    /exit        退出

  {C.BOLD}快捷键{C.RESET}
    Ctrl+C       中断当前生成    ↑ ↓         浏览历史输入
""")

    def _show_commands(self):
        print(f"""
  {C.BOLD}所有命令{C.RESET}

  {C.BOLD}对话管理{C.RESET}
    /new             开始新对话
    /save            保存当前对话
    /load [id]       加载历史对话
    /history         查看历史对话列表
    /clear           清空当前对话历史
    /compact         压缩对话（节省 token）

  {C.BOLD}设置{C.RESET}
    /model [name]    切换模型（opus4.6/opus4.5/sonnet4/sonnet3.5/haiku3.5）
    /status          查看连接状态

  {C.BOLD}系统{C.RESET}
    /help            显示帮助
    /commands        显示此列表
    /exit            退出

  {C.BOLD}使用示例{C.RESET}
    > 帮我看看这个项目的结构
    > 修复 src/main.py 里的 bug
    > git commit 并推送
    > 搜索关于 xxx 的飞书文档
    > 帮我记住：项目用 Python 3.11
""")

    def _switch_model(self, arg):
        if arg:
            key = MODEL_ALIASES.get(arg, arg)
            if key not in MODELS:
                err(f"未知模型: {arg}")
                dim("可用: " + ", ".join(MODELS.keys()))
                return
            self.config.model_key = key
            self.config.save_model()
            if self.client:
                self.client.config = self.config
                self.client.system_prompt = self.client._build_system_prompt()
            ok(f"已切换到 {self.config.model_name}")
        else:
            keys = list(MODELS.keys())
            # 找到当前模型的索引作为初始选中位置
            try:
                cur_idx = keys.index(self.config.model_key)
            except ValueError:
                cur_idx = 0

            selected = _arrow_select_model(keys, cur_idx)
            if selected is not None and 0 <= selected < len(keys):
                self.config.model_key = keys[selected]
                self.config.save_model()
                if self.client:
                    self.client.config = self.config
                    self.client.system_prompt = self.client._build_system_prompt()
                ok(f"已切换到 {self.config.model_name}")
            else:
                dim("  已取消")

    def _show_status(self):
        bridge_pid = _miramcp_running()
        print(f"\n  {C.BOLD}Mira CLI{C.RESET} {C.DIM}v{VERSION}{C.RESET}")
        print(f"  {C.DIM}{'─' * 40}{C.RESET}")
        a = f"{C.GREEN}✓{C.RESET}" if self.config.has_auth else f"{C.RED}✗{C.RESET}"
        m = f"{C.GREEN}✓{C.RESET}" if self.mcp else f"{C.DIM}–{C.RESET}"
        b = f"{C.GREEN}✓{C.RESET} {C.DIM}(PID: {bridge_pid}){C.RESET}" if bridge_pid else f"{C.DIM}–{C.RESET}"
        tool_count = len(self.client._tool_list) if self.client and self.client._tool_list else 0
        skill_count = len(self.client._skill_list) if self.client and self.client._skill_list else 0
        print(f"  LLM {a}  MCP {m}  Bridge {b}  Tools: {tool_count}  Skills: {skill_count}")
        print(f"  Model:  {C.BOLD}{self.config.model_name}{C.RESET} {C.DIM}({self.config.model_id}){C.RESET}")
        cwd = os.getcwd()
        home = str(Path.home())
        if cwd.startswith(home):
            cwd = "~" + cwd[len(home):]
        print(f"  Dir:    {cwd}")
        if self.client:
            print(f"  Chat:   {self.client.conv_id} {C.DIM}({len(self.client.messages)} msgs){C.RESET}")
        print()

    def _show_skills(self):
        """显示已启用的 Skill 列表"""
        if not self.client:
            warn("未初始化"); return
        skills = self.client.fetch_skill_list()
        tools = self.client.fetch_tool_list()
        print(f"\n  {C.BOLD}Skills & Tools{C.RESET}  {C.DIM}(client: {self.config.client_type}){C.RESET}")
        print(f"  {C.DIM}{'─' * 40}{C.RESET}")
        if skills:
            details = getattr(self.client, '_skill_details', [])
            market_skills = [d for d in details if d.get("type") == "market"]
            custom_skills = [d for d in details if d.get("type") == "custom"]
            if market_skills:
                print(f"  {C.BOLD}Market Skill ({len(market_skills)}){C.RESET}")
                for s in market_skills:
                    print(f"    {C.GREEN}✓{C.RESET} {s['name']} {C.DIM}({s['key']}){C.RESET}")
            if custom_skills:
                print(f"  {C.BOLD}Custom Skill ({len(custom_skills)}){C.RESET}")
                for s in custom_skills:
                    print(f"    {C.GREEN}✓{C.RESET} {s['name']} {C.DIM}({s['key']}){C.RESET}")
            if not details:
                # fallback: 没有 details 时直接显示 key
                print(f"  {C.BOLD}Skill ({len(skills)}){C.RESET}")
                for s in skills:
                    print(f"    {C.GREEN}✓{C.RESET} {s}")
        else:
            print(f"  {C.DIM}Skill: 无已启用的 Skill{C.RESET}")
        if tools:
            global_tools = [t for t in tools if t.get("scope") != "USER"]
            user_tools = [t for t in tools if t.get("scope") == "USER"]
            if global_tools:
                print(f"  {C.BOLD}Tools ({len(global_tools)}){C.RESET}")
                for t in global_tools:
                    print(f"    {C.GREEN}✓{C.RESET} {t['name']}")
            if user_tools:
                print(f"  {C.BOLD}User Tools ({len(user_tools)}){C.RESET}")
                for t in user_tools:
                    print(f"    {C.GREEN}✓{C.RESET} {t['name']}")
        print(f"\n  {C.DIM}管理 Skill: https://mira.byteintl.net/mira/settings/skills{C.RESET}")
        print()


    def _clear(self):
        if self.client: self.client.messages.clear()
        ok("对话已清空")

    def _new_conversation(self):
        if self.client and self.client.messages:
            self.client.save_conversation()
            dim(f"已保存会话 {self.client.conv_id}")
        if self.client:
            self.client.messages.clear()
            self.client.conv_id = str(uuid.uuid4())[:8]
        ok(f"新对话 {self.client.conv_id if self.client else ''}")

    def _save_conv(self):
        if self.client and self.client.messages:
            self.client.save_conversation()
            ok(f"已保存 ({self.client.conv_id}, {len(self.client.messages)} 条)")
        else:
            dim("没有可保存的对话")

    def _load_conv(self, arg):
        conv_id = arg.strip()
        if not conv_id:
            # 加载上次对话
            conv_id = ConversationStore.last_id()
            if not conv_id:
                dim("没有历史对话")
                return
        if self.client:
            if self.client.load_conversation(conv_id):
                ok(f"已加载会话 {conv_id} ({len(self.client.messages)} 条)")
            else:
                err(f"找不到会话: {conv_id}")

    def _list_convs(self):
        convs = ConversationStore.list_recent(10)
        if not convs:
            dim("没有历史对话")
            return
        print(f"\n  {C.BOLD}最近对话{C.RESET}\n")
        for c in convs:
            cur = " →" if self.client and c["id"] == self.client.conv_id else "  "
            print(f"  {cur}{C.BOLD}{c['id']}{C.RESET}  {C.DIM}{c['updated_at'][:16]}{C.RESET}  "
                  f"{c['messages']}条  {C.DIM}{c['preview']}{C.RESET}")
        print(f"\n  {C.DIM}使用 /load <id> 加载{C.RESET}\n")

    def _compact(self):
        if not self.client or len(self.client.messages) < 4:
            dim("无需压缩"); return
        old = len(self.client.messages)
        self.client.messages = self.client.messages[-6:]
        ok(f"压缩: {old} → {len(self.client.messages)} 条")

    def _exit(self):
        if self.client and self.client.messages:
            self.client.save_conversation()
            dim(f"已自动保存会话 {self.client.conv_id}")
        # 停止本地 MCP Bridge
        _auto_stop_mcp_bridge(getattr(self, '_mcp_proc', None))
        print(f"\n  {C.DIM}Bye 👋{C.RESET}\n")
        sys.exit(0)

    # ─── 响应处理 ───
    def process_response(self, stream: MiraSSEStream):
        blocks = []
        text_buf = ""
        thinking_buf = ""
        tool_use = None
        tool_json = ""
        stop_reason = None
        is_thinking = False
        _has_output = False  # 是否已经输出了任何文本内容
        _md = _MarkdownStreamer()  # 流式 Markdown 渲染器
        _spinner_active = False
        _had_reason_text = False  # reason 事件是否已流式输出了文本
        _from_content_buf = ""    # content(result) 的完整文本（用于去重）

        try:
            # 启动 thinking 旋转动画（隐藏光标避免闪烁，Coco 风格）
            _spinner_stop = threading.Event()
            def _spinner():
                frames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]
                i = 0
                sys.stdout.write(C.CURSOR_HIDE)
                sys.stdout.flush()
                while not _spinner_stop.is_set():
                    sys.stdout.write(f"\r  {C.PURPLE}⬡{C.RESET} {C.DIM}Thinking{C.RESET} {C.GRAY}{frames[i % len(frames)]}{C.RESET}  ")
                    sys.stdout.flush()
                    _spinner_stop.wait(0.1)
                    i += 1
                # 清除 spinner 行，恢复光标
                sys.stdout.write(f"\r\033[K{C.CURSOR_SHOW}")
                sys.stdout.flush()
            _spinner_thread = threading.Thread(target=_spinner, daemon=True)
            _spinner_thread.start()
            _spinner_active = True

            for evt in stream:
                t = evt.get("type", "")
                if t == "content_block_start":
                    # 跳过 content(result) 的合成 block（如果 reason 已输出过文本）
                    if evt.get("_from_content") and _had_reason_text:
                        continue
                    b = evt.get("content_block", {})
                    if b.get("type") == "text":
                        text_buf = ""
                        is_thinking = False
                    elif b.get("type") == "thinking":
                        # 先停止 spinner，再开始 thinking 输出（避免视觉冲突）
                        if _spinner_active:
                            _spinner_stop.set()
                            _spinner_thread.join(timeout=1)
                            _spinner_active = False
                        thinking_buf = ""
                        is_thinking = True
                        # 不输出 \n，直接在 spinner 清除后的同一行开始 thinking 内容
                    elif b.get("type") == "tool_use":
                        tool_name = b.get("name", "unknown")
                        # 简化 MCP 工具名：mcp__proxy__xxx__yyy__bash → bash
                        display_name = tool_name
                        if "__" in display_name:
                            display_name = display_name.rsplit("__", 1)[-1]
                        tool_use = {"id": b.get("id", ""), "name": tool_name}
                        tool_json = ""
                        is_thinking = False
                        # 先记住显示名，等 block_stop 时完整显示
                        tool_use["_display"] = display_name

                elif t == "content_block_delta":
                    d = evt.get("delta", {})
                    if d.get("type") == "text_delta":
                        txt = d.get("text", "")
                        is_from_content = evt.get("_from_content", False)
                        # content(result) 是聚合文本；若 reason 已流式输出过则跳过以避免重复
                        if is_from_content and _had_reason_text:
                            continue
                        if _spinner_active:
                            _spinner_stop.set()
                            _spinner_thread.join(timeout=1)
                            _spinner_active = False
                        if not is_from_content:
                            _had_reason_text = True
                        text_buf += txt
                        rendered = _md.feed(txt)
                        if rendered:
                            sys.stdout.write(rendered)
                            sys.stdout.flush()
                        _has_output = True
                    elif d.get("type") == "thinking_delta":
                        txt = d.get("text", "")
                        if _spinner_active:
                            _spinner_stop.set()
                            _spinner_thread.join(timeout=1)
                            _spinner_active = False
                        thinking_buf += txt
                        # thinking 内容不逐字输出（避免刷屏），只保持旋转提示
                    elif d.get("type") == "input_json_delta":
                        tool_json += d.get("partial_json", "")

                elif t == "content_block_stop":
                    # 跳过 content(result) 的合成 block stop（如果 reason 已输出过文本）
                    if evt.get("_from_content") and _had_reason_text:
                        continue
                    if is_thinking:
                        # thinking 结束：不多余换行，清除残留
                        sys.stdout.write(f"\r\033[K")
                        sys.stdout.flush()
                        is_thinking = False
                        thinking_buf = ""
                    elif text_buf:
                        # 刷出 markdown 渲染器中的剩余缓冲
                        remaining = _md.finish()
                        if remaining:
                            sys.stdout.write(remaining)
                            sys.stdout.flush()
                        blocks.append({"type": "text", "text": text_buf})
                        sys.stdout.write(f"{C.RESET}")
                        text_buf = ""
                    elif tool_use:
                        if _spinner_active:
                            _spinner_stop.set()
                            _spinner_thread.join(timeout=1)
                            _spinner_active = False
                        try: inp = json.loads(tool_json) if tool_json else {}
                        except Exception: inp = {}
                        # 工具显示前确保换行（避免跟文本粘在一行）
                        if _has_output:
                            sys.stdout.write("\n")
                        dn = tool_use.get("_display", tool_use["name"])
                        summary = _tool_summary(dn, inp)
                        if summary:
                            # 计算前缀 "  ● tool_name " 的可见宽度
                            prefix = f"  {C.ORANGE}●{C.RESET} {C.BOLD}{C.ORANGE}{dn}{C.RESET} "
                            prefix_vis = _display_width(dn) + 4  # "  ● name "
                            w = term_width()
                            remaining = max(20, w - prefix_vis)
                            if len(summary) <= remaining:
                                sys.stdout.write(f"{prefix}{C.GRAY}{summary}{C.RESET}\n")
                            else:
                                # 首行：前缀 + 命令开头
                                sys.stdout.write(f"{prefix}{C.GRAY}{summary[:remaining]}{C.RESET}\n")
                                # 续行：对齐到前缀宽度
                                rest = summary[remaining:]
                                pad = " " * prefix_vis
                                while rest:
                                    chunk = rest[:w - prefix_vis]
                                    rest = rest[len(chunk):]
                                    sys.stdout.write(f"{pad}{C.GRAY}{chunk}{C.RESET}\n")
                        else:
                            sys.stdout.write(f"  {C.ORANGE}●{C.RESET} {C.BOLD}{C.ORANGE}{dn}{C.RESET}\n")
                        sys.stdout.flush()
                        _has_output = True
                        blocks.append({"type": "tool_use", "id": tool_use["id"],
                                       "name": tool_use["name"], "input": inp})
                        tool_use = None; tool_json = ""
                        # 工具执行中，启动等待动画（Coco 风格）
                        if not _spinner_active:
                            _spinner_stop = threading.Event()
                            _tool_frames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]
                            def _tool_spinner(_stop=_spinner_stop, _frames=_tool_frames):
                                j = 0
                                sys.stdout.write(C.CURSOR_HIDE)
                                sys.stdout.flush()
                                while not _stop.is_set():
                                    sys.stdout.write(f"\r  {C.GRAY}{_frames[j % len(_frames)]}{C.RESET}  ")
                                    sys.stdout.flush()
                                    _stop.wait(0.1)
                                    j += 1
                                sys.stdout.write(f"\r\033[K{C.CURSOR_SHOW}")
                                sys.stdout.flush()
                            _spinner_thread = threading.Thread(target=_tool_spinner, daemon=True)
                            _spinner_thread.start()
                            _spinner_active = True

                elif t == "message_delta":
                    stop_reason = evt.get("delta", {}).get("stop_reason")
                elif t == "message_stop":
                    # 不 break！Mira 后端在 tool_use 后会继续发送新的 message，
                    # SSE 流通过 done:true 才真正结束（由迭代器 StopIteration 处理）
                    pass
                elif t == "error":
                    err(f"API: {evt.get('error', {}).get('message', '?')}")
                    break
        except KeyboardInterrupt:
            _spinner_stop.set()
            sys.stdout.write(C.CURSOR_SHOW)
            print(f"\n  {C.DIM}(interrupted){C.RESET}")
            stream.close()
        finally:
            if _spinner_active:
                _spinner_stop.set()
                _spinner_thread.join(timeout=1)
            sys.stdout.write(C.CURSOR_SHOW)
            sys.stdout.flush()
            stream.close()
            # 清理本次响应可能污染的 readline history
            self._sanitize_history()
            # 保证回复结束后有换行，避免和下一个 prompt 粘在一起
            print()
        return blocks, stop_reason

    def run_turn(self, user_input: str):
        try:
            stream = self.client.stream_chat(user_input)
            blocks, stop = self.process_response(stream)
            if blocks: self.client.add_assistant(blocks)

            # Mira 后端自己处理 tool_use（通过 local_mcp_server bridge），
            # SSE 流里的 tool_use 事件只是过程通知，不需要客户端执行。
            # 如果 stop_reason 是 tool_use 说明流被截断了（不应发生），
            # 不再重发请求避免 token 浪费。

            # 自动保存
            if len(self.client.messages) % 10 == 0:
                self.client.save_conversation()

        except AuthError as e: err(str(e))
        except APIError as e: err(str(e))
        except KeyboardInterrupt: print(f"\n  {C.DIM}(中断){C.RESET}")

    def _draw_prompt_header(self):
        """prompt 上方：分隔线 + 空行，清晰区分 AI 输出与用户输入"""
        w = min(term_width(), 90)
        bar = "─" * (w - 2)
        sys.stdout.write(f"\n  {C.DIVIDER}{bar}{C.RESET}\n\n")
        sys.stdout.flush()

    def _draw_prompt_footer(self):
        """prompt 下方：命令提示 + 状态信息（和 Coco 完全一致的布局）"""
        w = min(term_width(), 90)

        # 空行间隔
        sys.stdout.write("\n")
        # ─── 命令指引 ───
        sys.stdout.write(f"  {C.DIVIDER}💡{C.RESET} {C.GRAY}$/! shell  •  / command  •  \\↵ new line{C.RESET}\n")

        # ─── 状态栏（路径 + 模型）右对齐 ───
        cwd = os.getcwd()
        home = str(Path.home())
        if cwd.startswith(home):
            cwd = "~" + cwd[len(home):]
        short_path = os.path.basename(cwd) if cwd != "~" else "~"
        model_name = self.config.model_name if self.config else "?"
        r1 = f"📂 {short_path}"
        r2 = f"🤖 {model_name}"
        sys.stdout.write(f"{' ' * max(2, w - _display_width(r1))}{C.GREEN}{r1}{C.RESET}\n")
        sys.stdout.write(f"{' ' * max(2, w - _display_width(r2))}{C.PURPLE}{r2}{C.RESET}\n")
        sys.stdout.flush()

    def read_input(self) -> Optional[str]:
        # 清空 stdin 残留输入（用户在流式输出期间误按的按键）
        import termios
        try:
            termios.tcflush(sys.stdin.fileno(), termios.TCIFLUSH)
        except Exception:
            pass
        # 清空 readline 内部行缓冲区，防止上次对话恢复或流式输出的文本残留到新 prompt。
        # 使用 ctypes 调用 libreadline 的 rl_replace_line 彻底清除行缓冲，
        # 同时清除当前终端行的可见残留内容。
        def _clear_line():
            # 用 ctypes 彻底清除 readline 内部行缓冲区
            try:
                import ctypes, ctypes.util
                lib_name = ctypes.util.find_library("readline")
                if lib_name:
                    _rl = ctypes.cdll.LoadLibrary(lib_name)
                else:
                    _rl = None
                    for p in ("libreadline.so", "libreadline.so.8", "libreadline.so.6"):
                        try:
                            _rl = ctypes.cdll.LoadLibrary(p)
                            break
                        except OSError:
                            continue
                if _rl:
                    _rl.rl_replace_line(b"", 0)
                    _rl.rl_on_new_line()
            except Exception:
                pass
            sys.stdout.write("\r\033[K")
            sys.stdout.flush()
            readline.redisplay()
        readline.set_startup_hook(_clear_line)

        # 绘制分隔线
        self._draw_prompt_header()

        try: line = input(_rl_prompt(f"{C.BOLD}{C.GREEN}❯{C.RESET} "))
        except (EOFError, KeyboardInterrupt): return None
        finally:
            readline.set_startup_hook(None)

        # 收集所有输入行
        lines = [line]
        # 支持反斜杠续行
        while lines[-1].endswith("\\"):
            lines[-1] = lines[-1][:-1]
            try: lines.append(input(_rl_prompt(f"{C.GREEN}{C.DIM}…{C.RESET} ")))
            except: break
        # 检测粘贴缓冲区中的多行内容
        import select as _sel
        try:
            while _sel.select([sys.stdin], [], [], 0.05)[0]:
                extra = sys.stdin.readline()
                if not extra:
                    break
                lines.append(extra.rstrip('\n'))
        except Exception:
            pass
        text = "\n".join(lines)

        # 绘制命令提示 + 状态信息
        self._draw_prompt_footer()

        # 大段粘贴内容折叠显示（>20行）
        if len(lines) > 20:
            preview_head = lines[:3]
            preview_tail = lines[-2:]
            omitted = len(lines) - 5
            sys.stdout.write(f"\033[{len(lines)}A")  # 尝试上移光标
            sys.stdout.write(f"\033[J")  # 清除后续内容
            sys.stdout.write(f"{C.BOLD}{C.GREEN}❯{C.RESET} {preview_head[0]}\n")
            for pl in preview_head[1:3]:
                sys.stdout.write(f"  {C.DIM}{pl[:80]}{C.RESET}\n")
            sys.stdout.write(f"  {C.DIM}... ({omitted} lines omitted) ...{C.RESET}\n")
            for pl in preview_tail:
                sys.stdout.write(f"  {C.DIM}{pl[:80]}{C.RESET}\n")
            sys.stdout.flush()
        return text


    def interactive(self):
        self.ensure_auth()

        # 自动启动本地 MCP Bridge（后台）
        self._mcp_proc = _auto_start_mcp_bridge(self.config)

        # AI 编码贡献率：初始化 Tea 行级上报
        global _tea_reporter
        _tea_reporter = TeaReporter(username=self.config.username)

        dim("初始化 MCP 工具链...")
        mcp_ok = self._init_mcp()
        if mcp_ok:
            ok("MCP 已连接")
        else:
            warn("MCP 未连接（飞书/搜索/画图不可用，本地工具正常）")

        self.client = MiraClient(self.config, self.mcp)
        self.executor = ToolExecutor(self.mcp, self.client._mcp_tools_for_api, _tea_reporter)

        # 预加载后端工具列表（含 local_mcp_server 和 MCP 工具）
        tool_list = self.client.fetch_tool_list()
        if tool_list:
            mcp_tools = [t for t in tool_list if t.get("scope") != "USER"]
            user_tools = [t["name"] for t in tool_list if t.get("scope") == "USER"]
            if mcp_tools:
                ok(f"后端工具就绪 ({len(mcp_tools)} 个远程工具)")
            if user_tools:
                ok(f"本地工具已注入: {', '.join(user_tools)}")

        # 预加载已启用的 Skill 列表
        skill_list = self.client.fetch_skill_list()
        if skill_list:
            # 使用 display name 展示（如果有）
            if self.client._skill_details:
                names = [s["name"] for s in self.client._skill_details]
            else:
                names = skill_list
            ok(f"Skill 就绪 ({len(skill_list)}): {', '.join(names)}")

        # 尝试恢复上次对话
        last_id = ConversationStore.last_id()
        if last_id:
            try:
                # 临时关闭 readline 自动历史记录，避免恢复提示的输入污染 history
                readline.set_auto_history(False)
                ans = input(_rl_prompt(f"  {C.DIM}恢复上次对话 ({last_id})? [y/N]{C.RESET} ")).strip()
                readline.set_auto_history(True)
                if ans.lower() in ("y", "yes"):
                    if self.client.load_conversation(last_id):
                        ok(f"已恢复 ({len(self.client.messages)} 条)")
            except (EOFError, KeyboardInterrupt):
                readline.set_auto_history(True)

        self.banner()

        while True:
            try:
                user_input = self.read_input()
                if user_input is None:
                    # Ctrl+C / Ctrl+D → 直接退出
                    self._exit()
                user_input = user_input.strip()
                if not user_input: continue
                if user_input.startswith("/") and self.handle_slash(user_input): continue
                self.run_turn(user_input)
            except KeyboardInterrupt:
                print(f"\n  {C.DIM}(interrupted){C.RESET}")
                continue

    def single_query(self, query: str):
        self.ensure_auth()
        self._init_mcp()
        self.client = MiraClient(self.config, self.mcp)
        self.executor = ToolExecutor(self.mcp, self.client._mcp_tools_for_api, _tea_reporter)
        self.run_turn(query)
        print()


def do_ask(args) -> int:
    from mira_sidecar_main import main as sidecar_main

    return sidecar_main(args)


# ============================================================================
# MCP Bridge (miramcp) 管理
# ============================================================================

BOOTSTRAP_URL = "https://blade.byteintl.net/v1/admin/obj/bsave-agent-mycis/mira_agent_boostrap.sh"
MIRAMCP_DIR = MIRA_HOME / "miramcp"          # 工作目录
MIRAMCP_SCRIPTS = MIRAMCP_DIR / "scripts"     # bootstrap 脚本存放
MIRAMCP_PID_FILE = MIRAMCP_DIR / "miramcp.pid"


def _detect_miramcp_bin():
    """查找 miramcp 可执行文件"""
    # 1. 标准安装位置
    candidates = [
        Path.home() / ".local" / "bin" / "miramcp",
        Path.home() / ".local" / "bin" / "mira_cli",
    ]
    for c in candidates:
        if c.exists() and os.access(str(c), os.X_OK):
            return str(c)
    # 2. PATH 中查找
    for d in os.environ.get("PATH", "").split(os.pathsep):
        p = Path(d) / "miramcp"
        if p.exists() and os.access(str(p), os.X_OK):
            return str(p)
    return None


def _miramcp_running():
    """检查 miramcp 是否正在运行"""
    if MIRAMCP_PID_FILE.exists():
        try:
            pid = int(MIRAMCP_PID_FILE.read_text().strip())
            os.kill(pid, 0)  # 检测进程是否存在
            return pid
        except (ValueError, OSError):
            MIRAMCP_PID_FILE.unlink(missing_ok=True)
    return None


def _auto_start_mcp_bridge(config):
    """交互模式启动时自动拉起 miramcp（后台），返回 Popen 或 None"""
    if not config.device_id:
        return None
    bin_path = _detect_miramcp_bin()
    if not bin_path:
        return None
    # 已经在跑了就不重复启动
    if _miramcp_running():
        dim(f"MCP Bridge 已在运行 (device: {config.device_id})")
        return None
    # 释放端口
    try:
        subprocess.run(["lsof", "-ti:9801"], capture_output=True, text=True, timeout=3)
    except Exception:
        pass

    cmd = [bin_path, "run", "--device-id", config.device_id]
    try:
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        MIRAMCP_DIR.mkdir(parents=True, exist_ok=True)
        MIRAMCP_PID_FILE.write_text(str(proc.pid))
        # 等一小会让服务起来
        time.sleep(1)
        if proc.poll() is not None:
            warn("MCP Bridge 启动失败")
            return None
        dim(f"MCP Bridge 已后台启动 (PID: {proc.pid}, device: {config.device_id})")
        return proc
    except Exception as e:
        warn(f"MCP Bridge 启动失败: {e}")
        return None


def _auto_stop_mcp_bridge(proc):
    """退出时停掉我们自己启动的 miramcp 进程"""
    if proc is None:
        return
    try:
        proc.terminate()
        proc.wait(timeout=5)
    except Exception:
        try:
            proc.kill()
        except Exception:
            pass
    MIRAMCP_PID_FILE.unlink(missing_ok=True)


def do_mcp(args):
    """MCP Bridge 管理: setup / run / stop / status"""
    config = Config()

    sub = args[0] if args else ""

    # ── mira mcp setup ──
    if sub == "setup":
        _mcp_setup(config, args[1:])
        return

    # ── mira mcp stop ──
    if sub == "stop":
        _mcp_stop()
        return

    # ── mira mcp status ──
    if sub == "status":
        _mcp_status(config)
        return

    # ── mira mcp run [--device-id ID] ──
    if sub == "run":
        _mcp_run(config, args[1:], foreground=True)
        return

    # ── mira mcp (无参数) = setup + run 一键启动 ──
    if sub in ("", "start"):
        _mcp_start(config, args[1:] if sub == "start" else args)
        return

    # ── mira mcp --device-id ID (快捷设置并启动) ──
    if sub == "--device-id" and len(args) >= 2:
        config.device_id = args[1]
        config.save()
        ok(f"已保存 device-id: {config.device_id}")
        _mcp_start(config, [])
        return

    err(f"未知 mcp 子命令: {sub}")
    print(f"""
  {C.BOLD}用法{C.RESET}
    mira mcp                  一键安装并启动 MCP Bridge
    mira mcp setup            仅安装/更新 miramcp
    mira mcp run              前台运行（Ctrl+C 停止）
    mira mcp start            后台启动
    mira mcp stop             停止后台进程
    mira mcp status           查看状态

  {C.BOLD}选项{C.RESET}
    --device-id ID            设置设备标识（首次需要，之后记住）
""")


def _mcp_setup(config, extra_args):
    """下载并安装 miramcp (bootstrap)"""
    # 解析 --device-id
    device_id = _parse_device_id(extra_args) or config.device_id
    if device_id and device_id != config.device_id:
        config.device_id = device_id
        config.save()

    MIRAMCP_DIR.mkdir(parents=True, exist_ok=True)
    MIRAMCP_SCRIPTS.mkdir(parents=True, exist_ok=True)

    # 下载 bootstrap 脚本
    bootstrap_file = MIRAMCP_SCRIPTS / "mira_agent_boostrap.sh"
    print(f"  {C.DIM}下载 bootstrap 脚本...{C.RESET}")
    try:
        req = urllib.request.Request(BOOTSTRAP_URL, method="GET")
        resp = urllib.request.urlopen(req, timeout=30)
        data = resp.read()
        bootstrap_file.write_bytes(data)
        bootstrap_file.chmod(0o755)
    except Exception as e:
        err(f"下载 bootstrap 失败: {e}")
        return False

    # 运行 bootstrap 脚本（非 piped 模式，直接执行）
    print(f"  {C.DIM}运行安装脚本...{C.RESET}")
    env = os.environ.copy()
    if device_id:
        env["MIRA_DEVICE_ID"] = device_id
    try:
        proc = subprocess.run(
            ["bash", str(bootstrap_file)],
            cwd=str(MIRAMCP_SCRIPTS),
            env=env,
            timeout=300,
        )
        if proc.returncode != 0:
            err(f"安装脚本退出码: {proc.returncode}")
            return False
    except subprocess.TimeoutExpired:
        err("安装超时（5分钟）")
        return False
    except Exception as e:
        err(f"安装失败: {e}")
        return False

    # 验证安装
    bin_path = _detect_miramcp_bin()
    if bin_path:
        ok(f"miramcp 已安装: {bin_path}")
        # source env 文件，更新 PATH
        env_file = Path.home() / ".miramcp" / "env"
        if env_file.exists():
            # 解析 export PATH="..." 行，加到当前进程 PATH
            for line in env_file.read_text().splitlines():
                m = re.match(r'export PATH="([^"]+):\$PATH"', line)
                if m:
                    extra = m.group(1)
                    if extra not in os.environ.get("PATH", ""):
                        os.environ["PATH"] = extra + os.pathsep + os.environ.get("PATH", "")
        return True
    else:
        err("安装完成但未找到 miramcp 可执行文件")
        dim("请检查 ~/.local/bin/ 目录")
        return False


def _mcp_run(config, extra_args, foreground=True):
    """运行 miramcp"""
    device_id = _parse_device_id(extra_args) or config.device_id
    if device_id and device_id != config.device_id:
        config.device_id = device_id
        config.save()

    if not device_id:
        err("需要 device-id，首次运行请指定:")
        dim("  mira mcp --device-id <YOUR_ID>")
        dim(f"  例如: mira mcp --device-id {config.username or 'your-username'}_mac")
        return

    bin_path = _detect_miramcp_bin()
    if not bin_path:
        err("miramcp 未安装，请先运行: mira mcp setup")
        return

    # 停止已有进程
    _mcp_stop(quiet=True)

    cmd = [bin_path, "run", "--device-id", device_id]
    print(f"\n  {C.BOLD}{C.CYAN}启动 MCP Bridge{C.RESET}")
    dim(f"  命令: {' '.join(cmd)}")
    dim(f"  Device-ID: {device_id}")
    print()

    if foreground:
        # 前台模式，Ctrl+C 退出
        try:
            proc = subprocess.Popen(cmd)
            proc.wait()
        except KeyboardInterrupt:
            proc.terminate()
            proc.wait(timeout=5)
            print()
            ok("MCP Bridge 已停止")
    else:
        # 后台模式
        proc = subprocess.Popen(
            cmd,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        MIRAMCP_DIR.mkdir(parents=True, exist_ok=True)
        MIRAMCP_PID_FILE.write_text(str(proc.pid))
        ok(f"MCP Bridge 已后台启动 (PID: {proc.pid})")
        dim(f"  停止: mira mcp stop")


def _mcp_start(config, extra_args):
    """一键 setup + 后台启动"""
    device_id = _parse_device_id(extra_args) or config.device_id
    if device_id and device_id != config.device_id:
        config.device_id = device_id
        config.save()

    if not device_id:
        err("首次使用需要 device-id:")
        dim(f"  mira mcp --device-id {config.username or 'your-username'}_mac")
        return

    bin_path = _detect_miramcp_bin()
    if not bin_path:
        print(f"  {C.BOLD}未检测到 miramcp，开始安装...{C.RESET}\n")
        if not _mcp_setup(config, []):
            return
        bin_path = _detect_miramcp_bin()
        if not bin_path:
            return

    _mcp_run(config, [], foreground=True)


def _mcp_stop(quiet=False):
    """停止后台 miramcp 进程"""
    pid = _miramcp_running()
    if pid:
        try:
            os.kill(pid, 15)  # SIGTERM
            if not quiet:
                ok(f"已停止 MCP Bridge (PID: {pid})")
        except OSError:
            pass
        MIRAMCP_PID_FILE.unlink(missing_ok=True)
    else:
        # 也尝试杀 9801 端口的进程
        try:
            subprocess.run(
                ["lsof", "-ti:9801"],
                capture_output=True, text=True, timeout=5,
            )
        except Exception:
            pass
        if not quiet:
            dim("MCP Bridge 未在运行")


def _mcp_status(config):
    """显示 MCP Bridge 状态"""
    bin_path = _detect_miramcp_bin()
    pid = _miramcp_running()

    print(f"\n  {C.BOLD}MCP Bridge 状态{C.RESET}\n")
    print(f"  miramcp:    {'✓ ' + bin_path if bin_path else '✗ 未安装'}")
    print(f"  device-id:  {config.device_id or '未设置'}")
    print(f"  运行状态:    {'✓ 运行中 (PID: ' + str(pid) + ')' if pid else '✗ 未运行'}")

    # 检查端口
    try:
        result = subprocess.run(
            ["lsof", "-ti:9801"],
            capture_output=True, text=True, timeout=5,
        )
        port_pids = result.stdout.strip()
        if port_pids:
            print(f"  端口 9801:  ✓ 被进程 {port_pids} 占用")
        else:
            print(f"  端口 9801:  ✗ 空闲")
    except Exception:
        print(f"  端口 9801:  ? 无法检测")
    print()


def _parse_device_id(args):
    """从参数中提取 --device-id"""
    for i, a in enumerate(args):
        if a == "--device-id" and i + 1 < len(args):
            return args[i + 1]
        if a.startswith("--device-id="):
            return a.split("=", 1)[1]
    return ""


def do_update():
    """自更新: 从 TOS 下载最新版 mira.py"""
    # 允论的更新源域名白名单（仅字节内部域名）
    ALLOWED_HOSTS = ("tosv.byted.org", "tos-s3-cn-beijing.volces.com",
                     "bytedance.larkoffice.com", "bytedance.feishu.cn",
                     "byteintl.net")
    update_url = DEFAULT_UPDATE_URL
    if UPDATE_URL_FILE.exists():
        update_url = UPDATE_URL_FILE.read_text().strip()
    if not update_url:
        err("未配置更新源")
        dim("请先设置更新地址:")
        dim(f"  echo 'https://tosv.byted.org/obj/<bucket>/mira.py' > {UPDATE_URL_FILE}")
        dim("或重新运行安装命令来更新")
        return
    # 校验 URL 域名白名单
    try:
        parsed = urllib.parse.urlparse(update_url)
        host = parsed.hostname or ""
        if parsed.scheme != "https":
            err("更新 URL 必须使用 HTTPS"); return
        if not any(host == d or host.endswith("." + d) for d in ALLOWED_HOSTS):
            err(f"更新 URL 域名不在白名单: {host}")
            dim("允许: " + ", ".join(ALLOWED_HOSTS)); return
    except Exception:
        err("更新 URL 格式无效"); return
    print(f"  {C.DIM}正在检查更新...{C.RESET}")
    try:
        req = urllib.request.Request(update_url, method="GET")
        resp = urllib.request.urlopen(req, timeout=30)
        new_code = resp.read()
        # 验证是有效 Python 脚本
        if not new_code.startswith(b"#!/usr/bin/env python3"):
            err("下载的文件不是有效的 mira.py")
            return
        # 提取版本号
        m = re.search(rb'VERSION\s*=\s*"([^"]+)"', new_code)
        new_ver = m.group(1).decode() if m else "unknown"
        if new_ver == VERSION:
            ok(f"已是最新版 v{VERSION}")
            return
        mira_py = MIRA_HOME / "mira.py"
        # 备份当前版本
        backup = MIRA_HOME / f"mira.py.bak.{VERSION}"
        if mira_py.exists():
            shutil.copy2(str(mira_py), str(backup))
        mira_py.write_bytes(new_code)
        mira_py.chmod(0o600)
        ok(f"已更新: v{VERSION} → v{new_ver}")
        dim(f"备份: {backup}")
    except Exception as e:
        err(f"更新失败: {e}")


# ============================================================================
# 主入口
# ============================================================================

def main():
    MIRA_HOME.mkdir(parents=True, exist_ok=True)
    MIRA_HOME.chmod(0o700)  # 仅 owner 可访问
    args = sys.argv[1:]

    if not args:
        MiraCLI().interactive()
        return

    cmd = args[0]
    if cmd in ("help", "--help", "-h"):
        print(f"""
  {C.BOLD}{C.CYAN}Mira CLI{C.RESET} v{VERSION} — 字节内部 AI 编程助手

  {C.BOLD}用法{C.RESET}
    mira                 交互模式
    mira "问题"          单次提问
    mira login           飞书登录
    mira ask             Sidecar 批处理模式
    mira model [name]    切换模型
    mira mcp             MCP Bridge 管理（安装/启动/停止）
    mira update          检查并更新到最新版
    mira status          查看状态
    mira history         历史对话
    mira help            帮助

  {C.BOLD}全局选项{C.RESET}

  {C.BOLD}可用模型{C.RESET}
    opus4.6    Cloud-O-4.6       (默认，最强)
    opus4.6t   Cloud-O-4.6 Think
    opus4.5    Cloud-O-4.5
    sonnet4.6  Cloud-S-4.6
    sonnet4    Cloud-S-4
    sonnet3.7  Cloud-S-3.7
    sonnet3.5  Cloud-S-3.5
    haiku3.5   Cloud-H-3.5       (最快)
    gpt5.4     GPT-5.4
    gemini3.1  Gemini 3.1 Pro
    glm5       Glm-5

  {C.BOLD}交互命令{C.RESET}
    /help  /model  /status  /clear  /new  /save  /load  /history  /exit

  {C.BOLD}功能{C.RESET}
    • 本地命令执行、文件读写编辑、Git 操作
    • 飞书文档读写、知识库搜索（需 MCP 认证）
    • 网络搜索、AI 画图（需 MCP 认证）
    • 长期记忆存储和检索（需 MCP 认证）
    • 对话持久化（自动保存/恢复）
    • 模型实时切换
""")
    elif cmd in ("version", "--version", "-v"):
        print(f"mira v{VERSION}")
    elif cmd == "login":
        config = Config()
        # mira login --cookie "..." or mira login --curl "..."
        if len(args) >= 3 and args[1] in ("--cookie", "--curl", "-c"):
            raw = " ".join(args[2:])
            _save_login_cookie(config, raw)
        else:
            do_login(config)
    elif cmd == "update":
        do_update()
    elif cmd == "mcp":
        do_mcp(args[1:])
    elif cmd == "ask":
        sys.exit(do_ask(args[1:]))
    elif cmd == "model":
        cli = MiraCLI()
        cli._switch_model(args[1] if len(args) > 1 else "")
        cli._show_status()
    elif cmd == "status":
        MiraCLI()._show_status()
    elif cmd == "history":
        convs = ConversationStore.list_recent(20)
        if not convs: dim("没有历史对话")
        else:
            print(f"\n  {C.BOLD}历史对话{C.RESET}\n")
            for c in convs:
                print(f"  {C.BOLD}{c['id']}{C.RESET}  {C.DIM}{c['updated_at'][:16]}{C.RESET}  "
                      f"{c['messages']}条  {C.DIM}{c['preview']}{C.RESET}")
            print()
    elif cmd.startswith("-"):
        err(f"未知: {cmd}"); print("  mira help")
    else:
        MiraCLI().single_query(" ".join(args))

if __name__ == "__main__":
    main()
