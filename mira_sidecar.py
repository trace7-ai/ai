from contextlib import nullcontext

from mira_file_access import FileAccessor
from mira_roles import get_role
from mira_session import ACTIVE_SESSION_STATUS, SessionStore
from sidecar_contract import build_error_response
from sidecar_runner import EXIT_INVALID_REQUEST, execute_request


class SidecarService:
    def __init__(self, config_factory, client_factory, session_store=None):
        self.config_factory = config_factory
        self.client_factory = client_factory
        self.session_store = session_store or SessionStore()

    def _session_scope(self, request: dict):
        if request["session"]["mode"] != "sticky":
            return nullcontext()
        return self.session_store.lock(request["session"]["session_id"])

    def _load_snapshot(self, request: dict):
        if request["session"]["mode"] != "sticky":
            return None
        return self.session_store.inspect(request["session"]["session_id"])

    def _prepare_client(self, config, snapshot):
        client = self.client_factory(config)
        if snapshot and snapshot.status == ACTIVE_SESSION_STATUS:
            client.mira_session_id = snapshot.record["remote_session_id"]
        return client

    def _with_attached_files(self, request: dict):
        role = get_role(request["role"])
        manifest = request["file_manifest"]
        if manifest["mode"] == "none":
            return request, []
        if role is None:
            raise ValueError(f"unsupported role: {request['role']}")
        if "file_read" not in role.allowed_capabilities:
            raise ValueError(f"role does not allow file_read: {request['role']}")
        workspace_root = request["session"]["context_hint"]["workspace_root"]
        files, files_read = FileAccessor(manifest, workspace_root).read_authorized_files()
        context = dict(request["context"])
        context["files"] = [*request["context"]["files"], *files]
        prepared_request = dict(request)
        prepared_request["context"] = context
        return prepared_request, files_read

    def _session_payload(self, session_id: str, record: dict | None, status: str, reason=None) -> dict:
        if not record:
            return {
                "session_id": session_id,
                "turn_index": 0,
                "status": status,
                "reason": reason,
            }
        return {
            "session_id": record["session_id"],
            "turn_index": record.get("turn_count", 0),
            "status": status,
            "reason": reason,
            "ttl_seconds": record.get("ttl_seconds"),
            "expires_at": record.get("expires_at"),
        }

    def _session_error(self, request: dict, snapshot, model_name: str, code: str, message: str):
        response = build_error_response(
            code,
            message,
            role=request.get("role"),
            request_id=request.get("request_id"),
            model=model_name,
        )
        session_id = request["session"]["session_id"]
        status = snapshot.status if snapshot else "error"
        record = snapshot.record if snapshot else None
        response["session"] = self._session_payload(session_id, record, status, message)
        return response, EXIT_INVALID_REQUEST

    def _reject_unusable_session(self, request: dict, snapshot, model_name: str):
        if request["session"]["mode"] != "sticky" or snapshot is None:
            return None
        if snapshot.status == "missing" or snapshot.status == ACTIVE_SESSION_STATUS:
            return None
        error_code = "session_expired" if snapshot.status == "expired" else "invalid_session"
        error_message = snapshot.reason or f"session is not usable: {snapshot.status}"
        return self._session_error(request, snapshot, model_name, error_code, error_message)

    def _finalize_sticky_response(self, request: dict, snapshot, client, response: dict, exit_code: int):
        if request["session"]["mode"] != "sticky":
            return response, exit_code
        session_id = request["session"]["session_id"]
        if response["status"] != "ok":
            if response["errors"] and response["errors"][0]["code"] == "invalid_session" and snapshot:
                invalid = self.session_store.mark_invalid(
                    session_id,
                    snapshot.record,
                    response["errors"][0]["message"],
                )
                response["session"] = self._session_payload(
                    session_id,
                    invalid,
                    invalid["status"],
                    invalid["last_error"],
                )
                return response, EXIT_INVALID_REQUEST
            record = snapshot.record if snapshot else None
            response["session"] = self._session_payload(session_id, record, "error")
            return response, exit_code
        record = self.session_store.build_record(
            request,
            client.mira_session_id,
            existing=snapshot.record if snapshot else None,
        )
        self.session_store.save(session_id, record)
        response["session"] = self._session_payload(session_id, record, record["status"])
        return response, exit_code

    def run(self, request: dict) -> tuple[dict, int]:
        config = self.config_factory()
        if not config.has_auth:
            raise ValueError("missing login cookie, run mira login first")
        with self._session_scope(request):
            snapshot = self._load_snapshot(request)
            rejected = self._reject_unusable_session(request, snapshot, config.model_name)
            if rejected is not None:
                return rejected
            client = self._prepare_client(config, snapshot)
            prepared_request, files_read = self._with_attached_files(request)
            response, exit_code = execute_request(client, prepared_request)
            response["files_read"] = files_read
            return self._finalize_sticky_response(
                request,
                snapshot,
                client,
                response,
                exit_code,
            )
