import http.client
import json
import ssl
import time


class MiraSSEStream:
    def __init__(self, response, conn=None):
        self.response = response
        self._conn = conn
        self._block_id = 0
        self._queued = []

    def __iter__(self):
        return self

    def close(self):
        self._close()

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

    def __next__(self):
        if self._queued:
            return self._queued.pop(0)
        while True:
            line = None
            for attempt in range(3):
                try:
                    line = self.response.readline()
                    break
                except ssl.SSLError:
                    if attempt < 2:
                        time.sleep(0.1 * (attempt + 1))
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
            if not line or line == ":keep-alive":
                continue
            if line.startswith("data: "):
                payload = line[6:]
            elif line.startswith("data:"):
                payload = line[5:]
            else:
                continue
            try:
                raw = json.loads(payload)
            except json.JSONDecodeError:
                continue
            if raw.get("done"):
                self._close()
                raise StopIteration
            if raw.get("error"):
                error = raw["error"]
                message = error.get("message", str(error)) if isinstance(error, dict) else str(error)
                return {"type": "error", "error": {"message": message}}
            message = raw.get("Message")
            if not message:
                if raw.get("code") == 0:
                    self._close()
                    raise StopIteration
                continue
            if isinstance(message, str):
                try:
                    message = json.loads(message)
                except json.JSONDecodeError:
                    continue
            event = message.get("event", "")
            data = message.get("data")
            if event == "reason" and isinstance(data, dict):
                inner = data.get("event")
                if not inner or not isinstance(inner, dict):
                    continue
                inner_type = inner.get("type", "")
                if inner_type == "content_block_start":
                    block = inner.get("content_block", {})
                    self._block_id += 1
                    return {
                        "type": "content_block_start",
                        "content_block": {"type": block.get("type", "text"), "id": block.get("id", f"block_{self._block_id}")},
                    }
                if inner_type == "content_block_delta":
                    delta = inner.get("delta", {})
                    if delta.get("type") == "input_json_delta":
                        return {"type": "content_block_delta", "delta": {"type": "input_json_delta", "partial_json": delta.get("partial_json", "")}}
                    text = delta.get("text", "")
                    if text:
                        return {"type": "content_block_delta", "delta": {"type": delta.get("type", "text_delta"), "text": text}}
                if inner_type == "content_block_stop":
                    return {"type": "content_block_stop"}
                if inner_type == "message_delta":
                    return {"type": "message_delta", "delta": inner.get("delta", {}), "usage": inner.get("usage", {})}
                continue
            if event == "content" and isinstance(data, dict):
                if data.get("type") != "result":
                    continue
                text = data.get("text", "")
                if text:
                    return {
                        "type": "content_block_delta",
                        "delta": {"type": "text_delta", "text": text},
                        "_from_content": True,
                    }
