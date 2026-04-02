import sys

from mira_sidecar import SidecarEntrypoint
from mira_sidecar_client import Config, SidecarMiraClient


def main(args=None) -> int:
    entrypoint = SidecarEntrypoint(
        args=list(args or []),
        config_factory=Config,
        client_factory=SidecarMiraClient,
    )
    return entrypoint.run()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
