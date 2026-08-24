"""In-pod reverse proxy that mints a fresh Ollama gateway JWT per request.

The workstation gateway expects header x-api-key, claim app=jobshout, HS256,
10-minute TTL — the same scheme as server/internal/gatewayauth. OpenCode is a
minutes-long agent loop, so a static token in its config would expire mid-review.
OpenCode talks to 127.0.0.1:11434; this proxy forwards to OLLAMA_HOST and signs
every hop.

When OLLAMA_JWT_SECRET is empty the proxy still forwards, unsigned — that is
correct for a plain local Ollama with no gateway.
"""

from __future__ import annotations

import json
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

import jwt

APP_NAME = "jobshout"
TOKEN_TTL_SECONDS = 10 * 60
DEFAULT_LISTEN = ("127.0.0.1", 11434)

_HOP_BY_HOP = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
    "host",
    "content-length",
}


def mint_gateway_token(secret: str, now: float | None = None) -> str:
    """HS256 JWT matching gatewayauth.Apply. Empty secret → empty token."""
    if not secret:
        return ""
    issued = int(now if now is not None else time.time())
    payload = {"app": APP_NAME, "iat": issued, "exp": issued + TOKEN_TTL_SECONDS}
    return jwt.encode(payload, secret, algorithm="HS256")


def _copy_request_headers(incoming, secret: str) -> list[tuple[str, str]]:
    headers: list[tuple[str, str]] = []
    for key, value in incoming.items():
        if key.lower() in _HOP_BY_HOP or key.lower() == "x-api-key":
            continue
        headers.append((key, value))
    token = mint_gateway_token(secret)
    if token:
        headers.append(("x-api-key", token))
    return headers


class _ProxyHandler(BaseHTTPRequestHandler):
    # Set on the class by start_proxy before serve_forever.
    upstream: str = ""
    secret: str = ""

    def log_message(self, format: str, *args) -> None:  # noqa: A003
        return  # OpenCode is chatty; keep sidecar logs for the job, not every token chunk.

    def do_GET(self) -> None:  # noqa: N802
        self._forward()

    def do_POST(self) -> None:  # noqa: N802
        self._forward()

    def do_HEAD(self) -> None:  # noqa: N802
        self._forward()

    def _forward(self) -> None:
        length = int(self.headers.get("Content-Length") or 0)
        body = self.rfile.read(length) if length else None
        url = self.upstream.rstrip("/") + self.path
        headers = _copy_request_headers(self.headers, self.secret)
        req = Request(url, data=body, method=self.command)
        for key, value in headers:
            req.add_header(key, value)
        try:
            with urlopen(req, timeout=600) as resp:
                self.send_response(resp.status)
                for key, value in resp.headers.items():
                    if key.lower() in _HOP_BY_HOP:
                        continue
                    self.send_header(key, value)
                self.end_headers()
                if self.command == "HEAD":
                    return
                while True:
                    chunk = resp.read(8192)
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    self.wfile.flush()
        except HTTPError as exc:
            payload = exc.read()
            self.send_response(exc.code)
            self.send_header("Content-Type", exc.headers.get("Content-Type", "text/plain"))
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
        except URLError as exc:
            message = json.dumps({"error": f"ollama upstream unreachable: {exc.reason}"}).encode()
            self.send_response(502)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(message)))
            self.end_headers()
            self.wfile.write(message)


def start_proxy(upstream: str, secret: str, listen: tuple[str, int] = DEFAULT_LISTEN) -> ThreadingHTTPServer:
    """Bind the proxy and serve in a daemon thread. Returns the server."""
    handler = type(
        "BoundOllamaProxyHandler",
        (_ProxyHandler,),
        {"upstream": upstream.rstrip("/"), "secret": secret},
    )
    server = ThreadingHTTPServer(listen, handler)
    thread = threading.Thread(target=server.serve_forever, name="ollama-jwt-proxy", daemon=True)
    thread.start()
    return server


def proxy_base_url(listen: tuple[str, int] = DEFAULT_LISTEN) -> str:
    return f"http://{listen[0]}:{listen[1]}"
