from __future__ import annotations

import jwt

from app.ollama_proxy import APP_NAME, TOKEN_TTL_SECONDS, _copy_request_headers, mint_gateway_token


def test_mint_empty_secret_is_empty():
    assert mint_gateway_token("") == ""


def test_mint_matches_gatewayauth_claims():
    token = mint_gateway_token("s3cret", now=1_700_000_000)
    payload = jwt.decode(token, "s3cret", algorithms=["HS256"], options={"verify_exp": False})
    assert payload["app"] == APP_NAME
    assert payload["iat"] == 1_700_000_000
    assert payload["exp"] == 1_700_000_000 + TOKEN_TTL_SECONDS


def test_proxy_headers_set_x_api_key_and_drop_host():
    class Headers(dict):
        def items(self):
            return super().items()

    incoming = Headers({"Host": "127.0.0.1:11434", "Content-Type": "application/json", "X-Api-Key": "stale"})
    outgoing = dict(_copy_request_headers(incoming, "s3cret"))
    assert "Host" not in outgoing and "host" not in {k.lower() for k in outgoing}
    assert outgoing["Content-Type"] == "application/json"
    payload = jwt.decode(outgoing["x-api-key"], "s3cret", algorithms=["HS256"])
    assert payload["app"] == "jobshout"
