"""Smoke-test the actual console launcher without contacting an account."""
import json
import os
import shutil
from pathlib import Path
import subprocess
import sys
import tempfile

launcher = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory() as directory:
    if os.name == "nt":
        # The console entry point locates BalanceHUD.exe beside itself;
        # exercise the actual launcher from a portable folder with spaces.
        unpacked = Path(directory) / "Balance HUD with spaces"
        unpacked.mkdir()
        shutil.copyfile(launcher.with_name("BalanceHUD.exe"), unpacked / "BalanceHUD.exe")
        launcher = Path(shutil.copyfile(launcher, unpacked / launcher.name))
    accounts = Path(directory) / "accounts"
    accounts.mkdir()
    env = dict(os.environ, AGY_AUTH_HOME=str(accounts))
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
