"""Smoke-test the actual console launcher without contacting an account."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

launcher = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory() as directory:
    env = dict(os.environ, AGY_AUTH_HOME=directory)
    version = subprocess.run([launcher, "--json", "version"], env=env,
                             check=True, capture_output=True, timeout=10)
    assert json.loads(version.stdout)["data"]["version"]

    env.update(AGY_AUTH_CLIENT_ID="fixture-client",
               AGY_AUTH_CLIENT_SECRET="fixture-secret")
    login = subprocess.run(
        [launcher, "--json", "login", "--monitor-only", "--no-browser"],
        env=env, input=b"\n", capture_output=True, timeout=10)
    # An empty authorization code must reach the helper and be rejected
    # before any exchange. This covers stdin, stdout, stderr and exit status.
    assert login.returncode == 1
    assert json.loads(login.stdout)["error"]["code"] == "oauth_error"
    assert b"Paste the authorization code" in login.stderr
