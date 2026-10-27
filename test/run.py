import argparse
import copy
import os
import subprocess
import sys
import threading
from pathlib import Path

from settings import LIVE_BASE_URL, MODES, load_settings


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=MODES)
    parser.add_argument("--language", choices=("both", "go", "python"), default="both")
    arguments = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    try:
        settings = load_settings(root, mode=arguments.mode)
    except ValueError as error:
        print(str(error), file=sys.stderr)
        return 2
    environment = dict(os.environ)
    environment.update({
        "JUSTETF_TEST_MODE": settings.mode,
        "JUSTETF_TEST_EMAIL": settings.email,
        "JUSTETF_TEST_PASSWORD": settings.password,
        "PYTHONPATH": os.pathsep.join((str(root / "test"), str(root / "python/src"))),
    })
    server = worker = None
    if settings.mode == "offline":
        sys.path.insert(0, str(root / "python/src"))
        from mock_server import ContractHandler, ProfileServer
        server = ProfileServer(("127.0.0.1", 0), ContractHandler)
        initial_state = copy.deepcopy((server.portfolios, server.alerts, server.filters, server.transactions))
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        base_url = f"http://127.0.0.1:{server.server_port}"
    else:
        base_url = LIVE_BASE_URL
    environment["JUSTETF_TEST_BASE_URL"] = base_url
    print(f"Test tier: {settings.mode}; target: {base_url}", flush=True)
    try:
        if arguments.language in ("both", "go"):
            result = subprocess.run(["go", "test", "-race", "./...", "-count=1", "-timeout", "10m"], cwd=root, env=environment)
            if result.returncode:
                return result.returncode
        if arguments.language in ("both", "python"):
            filename = "justetf-contract-test.dll" if os.name == "nt" else "libjustetf-contract-test.dylib" if sys.platform == "darwin" else "libjustetf-contract-test.so"
            library = root / "bin" / filename
            result = subprocess.run(["go", "build", "-buildmode=c-shared", "-o", str(library), "./cmd/justetf-native"], cwd=root, env=environment)
            if result.returncode:
                return result.returncode
            environment["JUSTETF_TEST_LIBRARY"] = str(library)
            result = subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", "test/python", "-v"], cwd=root, env=environment)
            if result.returncode:
                return result.returncode
        if server is not None and initial_state != (server.portfolios, server.alerts, server.filters, server.transactions):
            print("Offline cleanup failed: shared mock account state was not restored.", file=sys.stderr)
            return 1
        print("Selected tier passed. Offline results do not certify live compatibility.", flush=True)
        return 0
    finally:
        if server is not None:
            server.shutdown()
            worker.join()
            server.server_close()


if __name__ == "__main__":
    raise SystemExit(main())