package linuxpatch

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// OSInfo is detected remote OS.
type OSInfo struct {
	IDLike  string
	ID      string
	Version string
	Flavor  string // debian, rhel, suse, amazon, unknown
}

// DetectOS reads /etc/os-release over SSH.
func DetectOS(client *ssh.Client) (OSInfo, error) {
	out, err := Run(client, "cat /etc/os-release 2>/dev/null || true")
	if err != nil {
		return OSInfo{Flavor: "unknown"}, err
	}
	info := OSInfo{Flavor: "unknown"}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			info.ID = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		}
		if strings.HasPrefix(line, "ID_LIKE=") {
			info.IDLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), `"`)
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			info.Version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
		}
	}
	blob := strings.ToLower(info.ID + " " + info.IDLike)
	switch {
	case strings.Contains(blob, "debian") || strings.Contains(blob, "ubuntu"):
		info.Flavor = "debian"
	case strings.Contains(blob, "rhel") || strings.Contains(blob, "centos") || strings.Contains(blob, "fedora") || strings.Contains(blob, "rocky") || strings.Contains(blob, "alma"):
		info.Flavor = "rhel"
	case strings.Contains(blob, "amzn") || strings.Contains(blob, "amazon"):
		info.Flavor = "amazon"
	case strings.Contains(blob, "suse") || strings.Contains(blob, "sles"):
		info.Flavor = "suse"
	}
	return info, nil
}

// ListPendingUpdates returns upgradable package names (best-effort).
func ListPendingUpdates(client *ssh.Client, flavor string) ([]string, string, error) {
	var cmd string
	switch flavor {
	case "debian":
		cmd = "export DEBIAN_FRONTEND=noninteractive; apt-get update -qq 2>/dev/null; apt list --upgradable 2>/dev/null | tail -n +2 | awk -F/ '{print $1}' | head -n 40"
	case "rhel", "amazon":
		cmd = "(dnf check-update -q 2>/dev/null || yum check-update -q 2>/dev/null) | awk 'NF{print $1}' | head -n 40"
	case "suse":
		cmd = "zypper list-updates 2>/dev/null | awk '/^v |^i /{print $3}' | head -n 40"
	default:
		cmd = "echo unknown-flavor"
	}
	out, err := Run(client, cmd)
	pkgs := filterLines(out)
	return pkgs, strings.TrimSpace(out), err
}

// ApplyUpdates installs pending security/all updates for the flavor.
func ApplyUpdates(client *ssh.Client, flavor string) (string, error) {
	var cmd string
	switch flavor {
	case "debian":
		cmd = "export DEBIAN_FRONTEND=noninteractive; apt-get update -qq && apt-get -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold upgrade"
	case "rhel", "amazon":
		cmd = "if command -v dnf >/dev/null; then dnf -y upgrade; else yum -y update; fi"
	case "suse":
		cmd = "zypper --non-interactive update"
	default:
		return "", fmt.Errorf("unsupported flavor %s", flavor)
	}
	return Run(client, cmd)
}

// NeedsReboot checks common reboot-required markers.
func NeedsReboot(client *ssh.Client) bool {
	out, _ := Run(client, "test -f /var/run/reboot-required && echo yes || (needs-restarting -r >/dev/null 2>&1 && echo yes || echo no)")
	return strings.Contains(out, "yes")
}

// EnsureServices starts and enables listed systemd units if inactive.
func EnsureServices(client *ssh.Client, services []string) (ok, fixed []string, msgs []string) {
	for _, svc := range services {
		svc = strings.TrimSpace(svc)
		if svc == "" {
			continue
		}
		st, _ := Run(client, "systemctl is-active "+shellQuote(svc)+" 2>/dev/null || true")
		st = strings.TrimSpace(st)
		if st == "active" {
			ok = append(ok, svc)
			continue
		}
		out, err := Run(client, "systemctl enable --now "+shellQuote(svc)+" 2>&1 || systemctl start "+shellQuote(svc)+" 2>&1")
		if err != nil {
			msgs = append(msgs, svc+": "+strings.TrimSpace(out))
			continue
		}
		fixed = append(fixed, svc)
		ok = append(ok, svc)
	}
	return ok, fixed, msgs
}

func filterLines(out string) []string {
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") {
			continue
		}
		pkgs = append(pkgs, line)
	}
	return pkgs
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ParseServices splits a service list.
func ParseServices(raw string) []string {
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
