"""Scope tests.

These are the tests that matter most in this service. Everything else decides
how a scan is run; this decides whether a host gets attacked.
"""

import pytest

from app import config, scope


@pytest.fixture(autouse=True)
def no_real_dns(monkeypatch):
    """Resolve names from a table rather than the internet.

    Tests that depend on live DNS fail for reasons that have nothing to do with
    the code, and this code's whole job is to be trusted about DNS.
    """
    table = {
        "example.com": ("93.184.216.34",),
        "api.example.com": ("93.184.216.34",),
        "notexample.com": ("198.51.100.7",),
        "internal.example.com": ("10.0.0.5",),
        "sneaky.example.com": ("169.254.169.254",),
        "roundrobin.example.com": ("93.184.216.34", "10.0.0.9"),
        "juice.local": ("127.0.0.1",),
        "lab.example.com": ("192.168.50.10",),
    }

    def fake_resolve(host):
        try:
            import ipaddress
            return (str(ipaddress.ip_address(host)),)
        except ValueError:
            pass
        if host not in table:
            raise scope.Unresolvable(f"target host {host!r} does not resolve")
        return table[host]

    monkeypatch.setattr(scope, "_resolve", fake_resolve)


def parse(entries):
    return scope.parse_rules(entries)


# ─── the default ────────────────────────────────────────────────────────────

def test_empty_allowlist_refuses_everything():
    with pytest.raises(scope.OutOfScope, match="no targets are in scope"):
        scope.enforce("https://example.com", parse(""))


def test_empty_allowlist_refuses_even_a_plain_public_host():
    # The point being that "looks harmless" is not a reason to scan something.
    with pytest.raises(scope.OutOfScope):
        scope.enforce("example.com", parse(""))


# ─── host rules ─────────────────────────────────────────────────────────────

def test_exact_host_allowed():
    target = scope.enforce("https://example.com", parse("example.com"))
    assert target.host == "example.com"


def test_host_not_on_the_list_refused():
    with pytest.raises(scope.OutOfScope, match="not in scope"):
        scope.enforce("https://notexample.com", parse("example.com"))


def test_host_match_is_case_insensitive():
    assert scope.enforce("https://EXAMPLE.com", parse("example.com")).host == "example.com"


def test_trailing_dot_does_not_evade_the_list():
    # "example.com." is the same name to a resolver, so it must be to us.
    assert scope.enforce("https://example.com.", parse("example.com")).host == "example.com"


def test_port_qualified_rule_requires_that_port():
    rules = parse("example.com:8443")
    assert scope.enforce("https://example.com:8443", rules).port == 8443
    with pytest.raises(scope.OutOfScope):
        scope.enforce("https://example.com:9999", rules)


# ─── wildcards ──────────────────────────────────────────────────────────────

def test_wildcard_covers_subdomain_and_base():
    rules = parse("*.example.com")
    assert scope.enforce("https://api.example.com", rules).host == "api.example.com"
    assert scope.enforce("https://example.com", rules).host == "example.com"


def test_wildcard_does_not_cover_a_lookalike_domain():
    # The bug a bare endswith() would introduce: "notexample.com" ends with
    # "example.com" but is a completely different registration.
    with pytest.raises(scope.OutOfScope):
        scope.enforce("https://notexample.com", parse("*.example.com"))


# ─── URL rules ──────────────────────────────────────────────────────────────

def test_url_rule_grants_its_path_prefix():
    rules = parse("https://example.com/app")
    assert scope.enforce("https://example.com/app", rules)
    assert scope.enforce("https://example.com/app/login", rules)


def test_url_rule_refuses_a_sibling_path():
    rules = parse("https://example.com/app")
    with pytest.raises(scope.OutOfScope):
        scope.enforce("https://example.com/admin", rules)


def test_url_rule_refuses_a_path_that_merely_shares_a_prefix_string():
    # "/application" starts with "/app" as a string but is a different path.
    with pytest.raises(scope.OutOfScope):
        scope.enforce("https://example.com/application", parse("https://example.com/app"))


def test_url_rule_honours_scheme():
    with pytest.raises(scope.OutOfScope):
        scope.enforce("http://example.com/app", parse("https://example.com/app"))


# ─── network rules ──────────────────────────────────────────────────────────

def test_cidr_rule_allows_an_address_inside_it():
    assert scope.enforce("http://192.168.50.10", parse("192.168.50.0/24")).host == "192.168.50.10"


def test_cidr_rule_refuses_an_address_outside_it():
    with pytest.raises(scope.OutOfScope):
        scope.enforce("http://192.168.51.10", parse("192.168.50.0/24"))


def test_cidr_rule_allows_a_hostname_resolving_into_it():
    assert scope.enforce("https://lab.example.com", parse("192.168.50.0/24"))


# ─── gate 2: where the name actually points ─────────────────────────────────

def test_allowlisted_host_resolving_to_private_space_is_refused():
    # The SSRF pivot. The name is on the list; where it points is not.
    with pytest.raises(scope.OutOfScope, match="internal address"):
        scope.enforce("https://internal.example.com", parse("*.example.com"))


def test_allowlisted_host_resolving_to_metadata_is_refused():
    with pytest.raises(scope.OutOfScope, match="metadata"):
        scope.enforce("https://sneaky.example.com", parse("*.example.com"))


def test_round_robin_with_one_internal_address_is_refused():
    # Judged on the internal address, not on the public one that happens to be
    # acceptable. Otherwise DNS ordering decides whether scope is enforced.
    with pytest.raises(scope.OutOfScope, match="internal address"):
        scope.enforce("https://roundrobin.example.com", parse("*.example.com"))


def test_loopback_is_refused_unless_named():
    with pytest.raises(scope.OutOfScope, match="loopback"):
        scope.enforce("http://juice.local:3000", parse("juice.local"))


def test_loopback_is_allowed_when_named_explicitly():
    # The real local-testing case: a vulnerable app on this machine, permitted
    # deliberately by naming the address as well as the host.
    target = scope.enforce("http://juice.local:3000", parse("juice.local,127.0.0.1"))
    assert target.port == 3000


def test_a_hostname_rule_cannot_authorise_an_internal_address():
    # Only a network rule can. Whoever wrote the hostname did not necessarily
    # know where it pointed, which is the entire gap gate 2 exists to close.
    with pytest.raises(scope.OutOfScope):
        scope.enforce("https://internal.example.com", parse("internal.example.com"))


def test_network_rule_does_authorise_its_own_internal_range():
    assert scope.enforce("https://internal.example.com", parse("internal.example.com,10.0.0.0/8"))


def test_allow_private_targets_opens_the_second_gate(monkeypatch):
    monkeypatch.setattr(config, "ALLOW_PRIVATE_TARGETS", True)
    assert scope.enforce("https://internal.example.com", parse("*.example.com"))


# ─── malformed input ────────────────────────────────────────────────────────

def test_unresolvable_host_is_refused():
    with pytest.raises(scope.OutOfScope):
        scope.enforce("https://nowhere.invalid", parse("*.invalid"))


def test_empty_target_refused():
    with pytest.raises(scope.OutOfScope, match="empty"):
        scope.enforce("", parse("example.com"))


@pytest.mark.parametrize("target", ["file:///etc/passwd", "gopher://example.com", "ftp://example.com"])
def test_non_http_schemes_refused(target):
    with pytest.raises(scope.OutOfScope, match="scheme"):
        scope.enforce(target, parse("*.example.com"))


def test_unparseable_allowlist_entry_is_dropped_not_fatal():
    # A typo must narrow scope, never widen it, and never stop the service.
    rules = parse("example.com, , *., https://")
    assert [r.source for r in rules] == ["example.com"]


def test_describe_reports_what_is_in_scope():
    described = scope.describe()
    assert described["scans_anything"] is False
    assert "allowlist" in described
