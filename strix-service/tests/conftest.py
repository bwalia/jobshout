"""Test configuration.

The environment is set before any ``app`` module is imported, because config.py
reads it once at import — the same property that makes the service simple in
production makes it order-sensitive under test.
"""

import os
import tempfile

# A scratch runs directory, so importing the app never touches a real one.
os.environ.setdefault("STRIX_RUNS_DIR", tempfile.mkdtemp(prefix="strix-test-runs-"))
os.environ.setdefault("STRIX_JWT_SECRET", "")
os.environ.setdefault("STRIX_TARGET_ALLOWLIST", "")
os.environ.setdefault("STRIX_MAX_RUNTIME_SECONDS", "10")

import pytest  # noqa: E402

from app import scope, store as store_module  # noqa: E402
from app.runner import Runner  # noqa: E402


@pytest.fixture
def runs_dir(tmp_path):
    return tmp_path / "runs"


@pytest.fixture
def store(runs_dir):
    return store_module.Store(runs_dir)


@pytest.fixture
def runner(store):
    return Runner(store, max_concurrent=1, queue_max=4, max_runtime=10)


@pytest.fixture
def rules():
    """Build allowlist rules from a string, without touching module state."""
    return scope.parse_rules


@pytest.fixture
def fake_strix(tmp_path):
    """A stand-in for the Strix binary.

    Strix is not installed in CI and takes hours when it is, so the runner is
    tested against a script that reproduces the two things the runner actually
    depends on: an exit code, and a vulnerabilities.json in the working
    directory.
    """
    def build(*, exit_code=0, findings=None, output="", sleep=0.0, subdir="strix_runs/abc123"):
        import json
        import stat
        script = tmp_path / f"fake-strix-{exit_code}-{abs(hash((output, sleep, subdir)))}"
        payload = json.dumps({"vulnerabilities": findings or []})
        script.write_text(
            "#!/usr/bin/env bash\n"
            f"printf '%s\\n' {json.dumps(output)}\n"
            f"sleep {sleep}\n"
            f"mkdir -p '{subdir}'\n"
            f"cat > '{subdir}/vulnerabilities.json' <<'JSON'\n{payload}\nJSON\n"
            f"exit {exit_code}\n"
        )
        script.chmod(script.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)
        return str(script)
    return build
