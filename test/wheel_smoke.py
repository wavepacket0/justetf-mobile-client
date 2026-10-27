import os
import sys
import threading
from pathlib import Path


TEST_DIRECTORY = Path(__file__).resolve().parent
sys.path.insert(0, str(TEST_DIRECTORY))
os.environ.pop("JUSTETF_LIBRARY", None)

from justetf_mobile_client import Client
from mock_server import ContractHandler, ProfileServer


def main() -> None:
    server = ProfileServer(("127.0.0.1", 0), ContractHandler)
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        with Client(base_url=f"http://127.0.0.1:{server.server_port}") as client:
            profile = client.get_profile("IE00B5BMR087")
        assert profile["isin"] == "IE00B5BMR087", profile
        assert profile["name"] == "Synthetic S&P 500 ETF", profile
        print("Installed platform wheel loaded its bundled native library and passed the mock API call")
    finally:
        server.shutdown()
        worker.join()
        server.server_close()


if __name__ == "__main__":
    main()