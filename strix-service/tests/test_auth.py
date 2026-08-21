"""Auth tests — the gateway JWT, verified exactly as gatewayauth mints it."""

import time

import jwt
import pytest
from fastapi import HTTPException

from app import config
from app.auth import require_auth

SECRET = "shared-secret-for-tests-at-least-32-bytes-long"


def mint(secret=SECRET, *, app="jobshout", ttl=600, skew=0, alg="HS256"):
    now = int(time.time()) + skew
    claims = {"app": app, "iat": now, "exp": now + ttl}
    if app is None:
        claims.pop("app")
    return jwt.encode(claims, secret, algorithm=alg)


@pytest.fixture
def secret(monkeypatch):
    monkeypatch.setattr(config, "JWT_SECRET", SECRET)


def test_no_secret_configured_accepts_anonymously(monkeypatch):
    # The documented local-development behaviour, matching the Go client's
    # "a nil signer means talk to the service directly".
    monkeypatch.setattr(config, "JWT_SECRET", "")
    assert require_auth("") == "anonymous"


def test_valid_token_returns_the_app_name(secret):
    assert require_auth(mint()) == "jobshout"


def test_missing_header_is_401(secret):
    with pytest.raises(HTTPException) as exc:
        require_auth("")
    assert exc.value.status_code == 401


def test_wrong_secret_is_403(secret):
    with pytest.raises(HTTPException) as exc:
        require_auth(mint("a-different-secret-also-32-bytes-long-x"))
    assert exc.value.status_code == 403


def test_expired_token_names_the_clock_as_the_likely_cause(secret):
    with pytest.raises(HTTPException) as exc:
        require_auth(mint(ttl=-3600))
    assert exc.value.status_code == 403
    assert "clock" in exc.value.detail


def test_token_without_an_app_claim_is_403(secret):
    with pytest.raises(HTTPException) as exc:
        require_auth(mint(app=None))
    assert exc.value.status_code == 403
    assert "app claim" in exc.value.detail


def test_small_clock_skew_is_tolerated(secret):
    # The caller truncates iat to a whole second, so a verifier a few
    # milliseconds behind would otherwise reject a perfectly good token.
    assert require_auth(mint(skew=30)) == "jobshout"


def test_unsigned_token_is_rejected(secret):
    # alg=none is the classic JWT bypass; PyJWT must refuse it because we pin
    # algorithms=["HS256"] on decode.
    unsigned = jwt.encode({"app": "jobshout"}, None, algorithm="none")
    with pytest.raises(HTTPException) as exc:
        require_auth(unsigned)
    assert exc.value.status_code == 403


def test_garbage_is_rejected_rather_than_crashing(secret):
    with pytest.raises(HTTPException) as exc:
        require_auth("not-a-token-at-all")
    assert exc.value.status_code == 403
