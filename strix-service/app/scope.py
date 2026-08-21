"""Target scoping — which hosts this service is permitted to attack.

The image service takes a prompt. This one takes *a target to attack*, and that
difference is the whole reason this module exists. An unguarded instance is an
open scanning relay for anyone who reaches it: arbitrary third parties, hosts
inside the LAN it sits on, or the cloud metadata endpoint.

Enforcement lives here — on the machine that actually runs the scan — rather
than only in the calling API. The caller's check is defence in depth and can be
bypassed by anyone who obtains a token; this one cannot be bypassed at all,
because it is the code path that decides whether a subprocess is spawned.

Two independent gates, both of which must pass:

  1. The target matches an allowlist rule. An empty allowlist matches nothing.
  2. Every address the target resolves to is either public, or inside a range
     the allowlist named explicitly.

Gate 2 exists because gate 1 alone trusts DNS. ``staging.example.com`` may be a
perfectly legitimate allowlist entry and still resolve to 10.0.0.5 or to
169.254.169.254 — which is the shape of an SSRF pivot, not of a normal target.
"""

from __future__ import annotations

import ipaddress
import logging
import socket
from dataclasses import dataclass
from urllib.parse import urlsplit

from app import config

logger = logging.getLogger(__name__)

# The AWS/GCP/Azure instance metadata address. Covered by the link-local check
# below, but named separately because it earns its own error message: it is the
# single most valuable thing an attacker can point a scanner at.
METADATA_ADDRESS = ipaddress.ip_address("169.254.169.254")


class OutOfScope(Exception):
    """The target may not be scanned. Permanent — never worth retrying."""


class Unresolvable(OutOfScope):
    """The target's scope could not be established, so it is refused.

    A subclass of OutOfScope rather than a separate failure because the outcome
    is identical: something that cannot be shown to be in scope is out of it.
    """


@dataclass(frozen=True)
class Target:
    raw: str
    scheme: str
    host: str
    port: int | None
    path: str
    addresses: tuple[str, ...]

    @property
    def normalized(self) -> str:
        """The target as it will be handed to Strix."""
        netloc = self.host if self.port is None else f"{self.host}:{self.port}"
        if not self.scheme:
            return netloc
        return f"{self.scheme}://{netloc}{self.path}"


@dataclass(frozen=True)
class Rule:
    kind: str  # "host" | "wildcard" | "network" | "url"
    host: str = ""
    port: int | None = None
    path: str = ""
    scheme: str = ""
    network: ipaddress.IPv4Network | ipaddress.IPv6Network | None = None
    source: str = ""


def parse_rules(raw: str) -> list[Rule]:
    """Parse STRIX_TARGET_ALLOWLIST into rules.

    An entry that cannot be parsed is dropped with a warning rather than raising.
    A single typo in a comma-separated list must not stop the service booting —
    but it must also not silently widen scope, and dropping an entry only ever
    narrows it.
    """
    rules: list[Rule] = []
    for entry in (e.strip() for e in raw.split(",")):
        if not entry:
            continue
        try:
            rules.append(_parse_rule(entry))
        except ValueError as exc:
            logger.warning("ignoring unparseable allowlist entry %r: %s", entry, exc)
    return rules


def _parse_rule(entry: str) -> Rule:
    # CIDR or bare IP first: "10.0.0.0/24" and "10.0.0.5" are unambiguous, and
    # trying them before URL parsing avoids urlsplit's opinions about colons.
    try:
        return Rule(kind="network", network=ipaddress.ip_network(entry, strict=False), source=entry)
    except ValueError:
        pass

    if "://" in entry:
        parts = urlsplit(entry)
        if not parts.hostname:
            raise ValueError("URL has no host")
        return Rule(
            kind="url",
            scheme=parts.scheme.lower(),
            host=parts.hostname.lower(),
            port=parts.port,
            # A rule of "https://x/app" grants "https://x/app/…" but not
            # "https://x/other". A bare "https://x" grants the whole host.
            path=parts.path.rstrip("/"),
            source=entry,
        )

    if entry.startswith("*."):
        base = entry[2:].lower().strip(".")
        if not base:
            raise ValueError("wildcard has no base domain")
        return Rule(kind="wildcard", host=base, source=entry)

    host, _, port = entry.partition(":")
    host = host.lower().strip(".")
    if not host:
        raise ValueError("empty host")
    return Rule(kind="host", host=host, port=int(port) if port else None, source=entry)


def parse_target(raw: str) -> Target:
    """Split a target string into its parts and resolve it.

    Raises OutOfScope for anything malformed — before any allowlist matching,
    because a target that cannot be understood cannot be reasoned about.
    """
    raw = (raw or "").strip()
    if not raw:
        raise OutOfScope("target is empty")

    # A bare "example.com" is a host, not a path, but urlsplit reads it as one.
    # Prefixing a scheme for parsing keeps the caller from having to write URLs
    # for targets that are not URLs.
    parseable = raw if "://" in raw else f"//{raw}"
    parts = urlsplit(parseable)

    # Scheme before host, because a target like "file:///etc/passwd" has no host
    # at all — and "unsupported scheme" says what is wrong with it, where "no
    # host" merely says something is.
    scheme = parts.scheme.lower()
    if scheme and scheme not in ("http", "https"):
        raise OutOfScope(f"target {raw!r} uses unsupported scheme {scheme!r}")

    try:
        host = parts.hostname
        port = parts.port
    except ValueError as exc:  # malformed port
        raise OutOfScope(f"target {raw!r} is not a valid host or URL: {exc}") from exc

    if not host:
        raise OutOfScope(f"target {raw!r} has no host")

    # Normalised before it is resolved, so the name checked against the allowlist
    # and the name looked up are the same string. "example.com." and
    # "example.com" are one host to a resolver and must be one host here too.
    host = host.lower().strip(".")
    if not host:
        raise OutOfScope(f"target {raw!r} has no host")

    return Target(
        raw=raw,
        scheme=scheme,
        host=host,
        port=port,
        path=parts.path.rstrip("/"),
        addresses=_resolve(host),
    )


def _resolve(host: str) -> tuple[str, ...]:
    """Every address the host resolves to.

    *Every* address, not the first: a round-robin name with one public and one
    internal address must be judged on the internal one. An IP literal resolves
    to itself without a lookup.
    """
    try:
        return (str(ipaddress.ip_address(host)),)
    except ValueError:
        pass

    try:
        infos = socket.getaddrinfo(host, None, proto=socket.IPPROTO_TCP)
    except socket.gaierror as exc:
        raise Unresolvable(f"target host {host!r} does not resolve: {exc}") from exc

    addresses = {info[4][0] for info in infos}
    if not addresses:
        raise Unresolvable(f"target host {host!r} does not resolve")
    # Sorted so the same host produces the same record every time, which makes
    # two scans of one target comparable in the log.
    return tuple(sorted(addresses))


def _host_matches(target: Target, rule: Rule) -> bool:
    if rule.kind == "host":
        if rule.port is not None and rule.port != target.port:
            return False
        return target.host == rule.host
    if rule.kind == "wildcard":
        # A wildcard covers subdomains and the base itself: "*.example.com"
        # grants "api.example.com" and "example.com", but never "notexample.com"
        # — hence the leading dot rather than a bare endswith.
        return target.host == rule.host or target.host.endswith("." + rule.host)
    return False


def _url_matches(target: Target, rule: Rule) -> bool:
    if rule.kind != "url":
        return False
    if rule.host != target.host:
        return False
    if rule.scheme and target.scheme and rule.scheme != target.scheme:
        return False
    if rule.port is not None and rule.port != target.port:
        return False
    if not rule.path:
        return True
    return target.path == rule.path or target.path.startswith(rule.path + "/")


def _network_matches(target: Target, rule: Rule) -> bool:
    """True when *every* resolved address falls inside the rule's network.

    All, not any: a name resolving to one in-range and one out-of-range address
    is out of range. Granting it on the strength of the address that happens to
    be acceptable is how a scanner ends up pointed somewhere it was never
    allowed to go.
    """
    if rule.kind != "network" or rule.network is None:
        return False
    return all(
        ipaddress.ip_address(addr) in rule.network for addr in target.addresses
    )


def _explicitly_allowed(addr: ipaddress.IPv4Address | ipaddress.IPv6Address, rules: list[Rule]) -> bool:
    """Whether a network rule names this address, however narrowly.

    Only network rules count. A hostname rule cannot authorise an internal
    address, because the person who wrote the hostname did not necessarily know
    where it pointed — and that gap is exactly what gate 2 is for.
    """
    return any(
        rule.kind == "network" and rule.network is not None and addr in rule.network
        for rule in rules
    )


def _check_addresses(target: Target, rules: list[Rule]) -> None:
    for raw_addr in target.addresses:
        addr = ipaddress.ip_address(raw_addr)
        if _explicitly_allowed(addr, rules) or config.ALLOW_PRIVATE_TARGETS:
            continue

        if addr == METADATA_ADDRESS:
            raise OutOfScope(
                f"target {target.raw!r} resolves to the cloud metadata address "
                f"{addr} — refused"
            )
        if addr.is_loopback:
            raise OutOfScope(
                f"target {target.raw!r} resolves to loopback address {addr}; "
                f"add it to STRIX_TARGET_ALLOWLIST explicitly to scan it"
            )
        if addr.is_link_local:
            raise OutOfScope(
                f"target {target.raw!r} resolves to link-local address {addr} — refused"
            )
        if addr.is_private or addr.is_reserved or addr.is_multicast:
            raise OutOfScope(
                f"target {target.raw!r} resolves to internal address {addr}; "
                f"name that range in STRIX_TARGET_ALLOWLIST to scan it"
            )


def enforce(raw: str, rules: list[Rule] | None = None) -> Target:
    """Return the target if it may be scanned, else raise OutOfScope."""
    if rules is None:
        rules = RULES

    target = parse_target(raw)

    if not rules:
        raise OutOfScope(
            "no targets are in scope: STRIX_TARGET_ALLOWLIST is empty, so this "
            "service will not scan anything"
        )

    matched = next(
        (
            rule
            for rule in rules
            if _host_matches(target, rule) or _url_matches(target, rule) or _network_matches(target, rule)
        ),
        None,
    )
    if matched is None:
        raise OutOfScope(
            f"target {target.raw!r} is not in scope; "
            f"add it to STRIX_TARGET_ALLOWLIST to permit it"
        )

    _check_addresses(target, rules)

    logger.info(
        "target in scope: %s (host=%s addresses=%s rule=%s)",
        target.raw, target.host, ",".join(target.addresses), matched.source,
    )
    return target


# Parsed once at import, like every other setting.
RULES: list[Rule] = parse_rules(config.TARGET_ALLOWLIST)


def describe() -> dict:
    """What this service will scan, for /api/capabilities."""
    return {
        "allowlist": [rule.source for rule in RULES],
        "rule_count": len(RULES),
        "allow_private_targets": config.ALLOW_PRIVATE_TARGETS,
        "scans_anything": False,
    }
