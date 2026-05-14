#!/usr/bin/env python3
import os
import shutil
import subprocess
import sys


def main() -> int:
    binary = shutil.which("ai-agent-manager")
    if not binary:
        sys.stderr.write("ai-agent-manager is not installed on PATH\n")
        return 1

    env = os.environ.copy()
    server = subprocess.run([binary, "open", "--start-server"], env=env, check=False)
    return server.returncode


if __name__ == "__main__":
    raise SystemExit(main())
