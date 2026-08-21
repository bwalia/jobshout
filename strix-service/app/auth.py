"""Gateway authentication.

This verifies exactly what ``server/internal/gatewayauth`` mints: an HS256 JWT in
the ``x-api-key`` header — no ``Bearer`` prefix — carrying an ``app`` claim,
signed with a secret shared out of band. It is a deliberate copy of
``image-service/app/auth.py``: the platform now runs three workstation services
behind one scheme, and a third variation on it would be a third secret to rotate
and a third thing to get subtly wrong.

Use a *different secret* here than for Ollama and the image service. They front
a language model and a GPU; this one fronts a vulnerability scanner, and a leak
of one credential should not hand over all three.
"""

import logging

import jwt
from fastapi import Header, HTTPException, status

from app import config

logger = logging.getLogger(__name__)


def require_auth(x_api_key: str = Header(default="")) -> str:
    """Verify the gateway token, returning the calling app's name.

    With no secret configured every request is accepted and the caller is
    reported as ``"anonymous"`` — the same rule the Go client follows in reverse,
    where a nil signer means "talk to the service directly". It is what lets this
    run on a laptop without minting anything.

    Note what does *not* follow from that: an unauthenticated service still
    refuses every target outside its allowlist, because scope.py is a separate
    gate. Unset auth widens who may ask, never what may be scanned.
    """
    if not config.JWT_SECRET:
        return "anonymous"

    if not x_api_key:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="missing x-api-key header",
        )

    try:
        claims = jwt.decode(
            x_api_key,
            config.JWT_SECRET,
            algorithms=["HS256"],
            leeway=config.JWT_LEEWAY,
        )
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
