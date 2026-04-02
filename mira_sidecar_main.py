import sys

from mira_sidecar import SidecarService
from mira_sidecar_cli import load_request_from_cli
from mira_sidecar_client import Config, SidecarMiraClient
from sidecar_contract import build_error_response
from sidecar_runner import EXIT_INVALID_REQUEST, print_response


def main(args=None) -> int:
    try:
        request = load_request_from_cli(list(args or []))
        response, exit_code = SidecarService(
            config_factory=Config,
            client_factory=SidecarMiraClient,
        ).run(request)
    except Exception as exc:
        response = build_error_response("invalid_request", str(exc))
        exit_code = EXIT_INVALID_REQUEST
    print_response(response)
    return exit_code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
