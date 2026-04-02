from mira_file_access import FileAccessor
from mira_roles import ROLE_REGISTRY, get_role
from mira_session import SessionStore
from sidecar_contract import build_error_response
from sidecar_runner import EXIT_INVALID_REQUEST, execute_request, load_request_file, print_response

SESSION_ERROR_HINTS = (
    "invalid session",
    "session expired",
    "session not found",
    "conversation not found",
    "invalid conversation",
    "会话失效",
    "会话不存在",
    "会话已过期",
)


class SidecarEntrypoint:
    def __init__(self, args, config_factory, client_factory, session_store=None):
        self.args = args
        self.config_factory = config_factory
        self.client_factory = client_factory
        self.session_store = session_store or SessionStore()

    def _parse_args(self):
        request_path = ""
        output_format = "json"
        role_override = None
        index = 0
        while index < len(self.args):
            current = self.args[index]
            if current == "--input-file" and index + 1 < len(self.args):
                request_path = self.args[index + 1]
                index += 2
                continue
            if current == "--format" and index + 1 < len(self.args):
                output_format = self.args[index + 1]
                index += 2
                continue
            if current == "--role" and index + 1 < len(self.args):
                role_override = self.args[index + 1]
                index += 2
                continue
            raise ValueError(f"unknown ask argument: {current}")
        if not request_path:
            raise ValueError("missing --input-file <path>")
        if output_format != "json":
            raise ValueError("only --format json is supported in v1")
        return request_path, role_override

    def _load_request(self):
        request_path, role_override = self._parse_args()
        request = load_request_file(request_path)
        if role_override:
            if role_override not in ROLE_REGISTRY:
                raise ValueError(f"unsupported role override: {role_override}")
            request["role"] = role_override
        return request

    def _prepare_client(self, config, request: dict):
        client = self.client_factory(config)
        if request["session"]["mode"] != "sticky":
            return client, None
        session_id = request["session"]["session_id"]
        stored = self.session_store.load(session_id)
        if self.session_store.is_valid(stored):
            client.mira_session_id = stored["remote_session_id"]
        return client, stored

    def _attach_files(self, request: dict):
        role = get_role(request["role"])
        manifest = request["file_manifest"]
        if manifest["mode"] == "none":
            return []
        if role is None:
            raise ValueError(f"unsupported role: {request['role']}")
        if "file_read" not in role.allowed_capabilities:
            raise ValueError(f"role does not allow file_read: {request['role']}")
        workspace_root = request["session"]["context_hint"]["workspace_root"]
        files, files_read = FileAccessor(manifest, workspace_root).read_authorized_files()
        request["context"]["files"].extend(files)
        return files_read

    def _should_retry_session(self, request: dict, stored, response: dict) -> bool:
        if request["session"]["mode"] != "sticky":
            return False
        if not stored or not stored.get("remote_session_id"):
            return False
        if response["status"] != "error":
            return False
        message = " ".join(error.get("message", "") for error in response.get("errors", []))
        lowered = message.lower()
        return any(hint in lowered for hint in SESSION_ERROR_HINTS)

    def _run_request(self, client, request: dict, stored):
        response, exit_code = execute_request(client, request)
        if not self._should_retry_session(request, stored, response):
            return response, exit_code, False
        client.mira_session_id = ""
        retry_response, retry_exit_code = execute_request(client, request)
        return retry_response, retry_exit_code, True

    def _attach_session(self, response: dict, request: dict, client, stored, reconnected=False):
        if request["session"]["mode"] != "sticky":
            return
        if response["status"] != "ok":
            response["session"] = {
                "session_id": request["session"]["session_id"],
                "turn_index": (stored or {}).get("turn_count", 0),
                "status": "error",
                "reconnected": reconnected,
            }
            return
        if not client.mira_session_id:
            response["session"] = {
                "session_id": request["session"]["session_id"],
                "turn_index": (stored or {}).get("turn_count", 0),
                "status": "inactive",
                "reconnected": reconnected,
            }
            return
        record = self.session_store.build_record(
            request,
            client.mira_session_id,
            existing=stored,
            reset_turn_count=reconnected,
        )
        self.session_store.save(request["session"]["session_id"], record)
        response["session"] = {
            "session_id": record["session_id"],
            "turn_index": record["turn_count"],
            "status": "reconnected" if reconnected else record["status"],
            "reconnected": reconnected,
        }

    def run(self) -> int:
        try:
            request = self._load_request()
            config = self.config_factory()
            if not config.has_auth:
                raise ValueError("missing login cookie, run mira login first")
            client, stored = self._prepare_client(config, request)
            files_read = self._attach_files(request)
            response, exit_code, reconnected = self._run_request(client, request, stored)
            response["files_read"] = files_read
            self._attach_session(response, request, client, stored, reconnected=reconnected)
        except Exception as exc:
            response = build_error_response("invalid_request", str(exc))
            exit_code = EXIT_INVALID_REQUEST
        print_response(response)
        return exit_code
