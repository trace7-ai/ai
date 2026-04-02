import http.client
import json
import os
import ssl
import urllib.parse
from datetime import datetime
from pathlib import Path

from mira_sidecar_stream import MiraSSEStream

MIRA_HOME = Path(os.environ.get("MIRA_HOME", Path.home() / ".mira"))
CONFIG_FILE = MIRA_HOME / "config.json"
MODEL_FILE = MIRA_HOME / "model"
MIRA_BASE = "https://mira.byteintl.net"
DEFAULT_MODEL = "opus4.6"
MODELS = {
    "opus4.6": ("Cloud-O-4.6", "re-o-46", "quick"),
    "opus4.6t": ("Cloud-O-4.6 Think", "re-o-46", "deep"),
    "opus4.5": ("Cloud-O-4.5", "re-o-45", "quick"),
    "sonnet4.6": ("Cloud-S-4.6", "re-s-46", "quick"),
    "sonnet4": ("Cloud-S-4", "claude-sonnet-4-20250514", "quick"),
    "sonnet3.7": ("Cloud-S-3.7", "claude-3-7-sonnet-20250219", "quick"),
    "sonnet3.5": ("Cloud-S-3.5", "claude-3-5-sonnet-20241022", "quick"),
    "haiku3.5": ("Cloud-H-3.5", "claude-3-5-haiku-20241022", "quick"),
    "gpt5.4": ("GPT-5.4", "gpt-5.4", "quick"),
    "gemini3.1": ("Gemini 3.1 Pro", "gemini-3.1-pro-preview", "quick"),
    "glm5": ("Glm-5", "glm-5", "quick"),
}
_cookie_debug = os.environ.get("MIRA_DEBUG", "") == "1"


class AuthError(Exception):
    pass


class APIError(Exception):
    pass


def _cdebug(message: str):
    if _cookie_debug:
        print(f"[MIRA_DEBUG] {message}", file=os.sys.stderr)


def _safe_cookie(value: str) -> str:
    if not value:
        return ""
    cleaned = "".join(char for char in value if ord(char) >= 0x20 and ord(char) != 0x7F)
    return cleaned.encode("ascii", errors="ignore").decode("ascii").strip()


def _http_request(url, *, method="GET", headers=None, data=None, timeout=30):
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname
    port = parsed.port or (443 if parsed.scheme == "https" else 80)
    path = parsed.path + (f"?{parsed.query}" if parsed.query else "")
    if parsed.scheme == "https":
        conn = http.client.HTTPSConnection(
            host,
            port,
            timeout=timeout,
            context=ssl.create_default_context(),
        )
    else:
        conn = http.client.HTTPConnection(host, port, timeout=timeout)
    try:
        conn.request(method, path, body=data, headers=headers or {})
        response = conn.getresponse()
        return response.status, response.read(), dict(response.getheaders())
    finally:
        conn.close()


class Config:
    def __init__(self):
        self.cookies = ""
        self.username = ""
        self.device_id = ""
        self.model_key = DEFAULT_MODEL
        self._load()

    def _load(self):
        if CONFIG_FILE.exists():
            data = json.loads(CONFIG_FILE.read_text(encoding="utf-8"))
            if not isinstance(data, dict):
                raise ValueError(f"config file must contain a JSON object: {CONFIG_FILE}")
            self.cookies = data.get("cookies", "")
            if not self.cookies and data.get("session"):
                self.cookies = f"session={data['session']}"
            self.username = data.get("username", "")
            self.device_id = data.get("device_id", "")
        if MODEL_FILE.exists():
            key = MODEL_FILE.read_text(encoding="utf-8").strip()
            if key in MODELS:
                self.model_key = key

    @property
    def model_id(self):
        return MODELS[self.model_key][1]

    @property
    def model_name(self):
        return MODELS[self.model_key][0]

    @property
    def model_mode(self):
        return MODELS[self.model_key][2]

    @property
    def has_auth(self):
        return bool(self.cookies)


class SidecarMiraClient:
    def __init__(self, config: Config):
        self.config = config
        self.mira_session_id = ""

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

    def _ensure_session(self):
        if self.mira_session_id:
            return
        payload = {
            "sessionProperties": {
                "topic": "",
                "dataSource": "360_performance",
                "dataSources": ["manus"],
                "model": self.config.model_id,
            }
        }
        data = json.dumps(payload, ensure_ascii=True).encode("utf-8")
        headers = self._headers()
        headers["Content-Length"] = str(len(data))
        status, body, response_headers = _http_request(
            f"{MIRA_BASE}/mira/api/v1/chat/create",
            method="POST",
            headers=headers,
            data=data,
            timeout=30,
        )
        raw = body.decode(errors="replace")
        _cdebug(f"sidecar create session status={status} body_len={len(raw)}")
        if status == 401:
            raise AuthError("认证失效，请重新登录: mira login")
        if status >= 400:
            _cdebug(f"sidecar create session headers={response_headers}")
            try:
                parsed = json.loads(raw)
                if parsed.get("code") == 20001:
                    raise AuthError("认证失效，请重新登录: mira login")
            except (json.JSONDecodeError, ValueError):
                pass
            raise APIError(f"创建会话失败 {status}: {raw[:300]}")
        parsed = json.loads(raw)
        if parsed.get("code") == 20001:
            raise AuthError("认证失效，请重新登录: mira login")
        item = parsed.get("sessionItem", parsed.get("session_item", {}))
        session_id = item.get("sessionId", item.get("session_id", ""))
        if not session_id:
            raise APIError("创建会话失败: 响应中无 sessionId")
        self.mira_session_id = session_id

    def stream_chat(self, user_input: str):
        env_context = (
            f"\n\n[System Context] "
            f"cwd={os.getcwd()} | "
            f"platform={os.sys.platform} | "
            f"user={os.environ.get('USER', 'unknown')} | "
            f"model={self.config.model_name} | "
            f"time={datetime.now().strftime('%Y-%m-%d %H:%M')}"
        )
        return self._stream_request(user_input + env_context)

    def _stream_request(self, content: str):
        self._ensure_session()
        payload = {
            "sessionId": self.mira_session_id,
            "content": content,
            "messageType": 1,
            "summaryAgent": self.config.model_id,
            "dataSources": ["manus"],
            "comprehensive": 1,
            "config": {
                "online": True,
                "mode": self.config.model_mode,
            },
        }
        data = json.dumps(payload).encode()
        headers = self._headers()
        headers["Accept"] = "text/event-stream"
        headers["Content-Length"] = str(len(data))
        parsed = urllib.parse.urlparse(f"{MIRA_BASE}/mira/api/v1/chat/completion")
        conn = http.client.HTTPSConnection(
            parsed.hostname,
            443,
            timeout=300,
            context=ssl.create_default_context(),
        )
        try:
            conn.request("POST", parsed.path, body=data, headers=headers)
            response = conn.getresponse()
        except Exception as exc:
            conn.close()
            raise APIError(f"网络错误: {exc}")
        if response.status >= 400:
            body = response.read().decode(errors="replace")
            conn.close()
            try:
                parsed_body = json.loads(body)
                if parsed_body.get("code") == 20001:
                    raise AuthError("认证失效，请重新登录: mira login")
            except (json.JSONDecodeError, ValueError):
                pass
            if response.status == 401:
                raise AuthError("认证失效，请重新登录: mira login")
            raise APIError(f"API 错误 {response.status}: {body[:300]}")
        content_type = response.getheader("Content-Type", "")
        if "text/event-stream" not in content_type and "application/json" in content_type:
            body = response.read().decode(errors="replace")
            conn.close()
            try:
                parsed_body = json.loads(body)
                if parsed_body.get("code") == 20001:
                    raise AuthError("认证失效，请重新登录: mira login")
                raise APIError(f"服务端错误 (code={parsed_body.get('code', 0)}): {parsed_body.get('msg', 'unknown error')}")
            except (json.JSONDecodeError, ValueError):
                raise APIError(f"非预期响应 (Content-Type: {content_type}): {body[:300]}")
        return MiraSSEStream(response, conn)
