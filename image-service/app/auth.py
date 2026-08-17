"""Gateway authentication.

This verifies exactly what ``server/internal/llm/ollama_auth.go`` mints: an
HS256 JWT in the ``x-api-key`` header — no ``Bearer`` prefix — carrying an
``app`` claim, signed with a secret shared out of band. Keeping the two in
lockstep is the point: the platform already runs one workstation model service
behind this scheme, and a second one that authenticated differently would be a
second secret, a second rotation and a second thing to get wrong.
"""

import logging

import jwt
from fastapi import Header, HTTPException, status

from app import config

logger = logging.getLogger(__name__)


def require_auth(x_api_key: str = Header(default="")) -> str:
    """Verify the gateway token, returning the calling app's name.

    With no secret configured every request is accepted and the caller is
    reported as ``"anonymous"``. That is the same rule the Go client follows in
    reverse — a nil auth is valid and means "talk to the service directly" — and
    it is what lets a developer run this on a laptop without minting anything.
    It is only safe because the service then has no secret worth protecting; on
    a host reachable from off the machine, set ``IMAGE_JWT_SECRET``.
    """
    if not config.JWT_SECRET:
        return "anonymous"

    if not x_api_key:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="missing x-api-key header",
        )

    try:
        claims = jwt.decode(x_api_key, config.JWT_SECRET, algorithms=["HS256"])
    except jwt.ExpiredSignatureError:
        # Called out separately from a bad signature because the fix is
        # different: an expired token from a correctly configured caller means
        # the two machines disagree about the time, not about the secret.
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="token expired — check the clock on the calling host",
        )
    except jwt.InvalidTokenError as exc:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail=f"token did not verify: {exc}",
        )

    app_name = claims.get("app")
    if not app_name:
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="token carries no app claim",
        )
    return str(app_name)
