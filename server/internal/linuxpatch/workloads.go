package linuxpatch

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
	"golang.org/x/crypto/ssh"
)

const WSLVaultDocs = "https://www.wslvault.org/"

// WorkloadProfile describes HA-aware pre/post behaviour.
type WorkloadProfile struct {
	Name         string
	ZeroDowntime bool
	Hints        []string
	DefaultSvcs  []string
	DocsURL      string
	PreCommands  func(host string, idx, total int) []string
	PostCommands func(host string, idx, total int) []string
	HealthCheck  func(client *ssh.Client) (string, error)
}

// ProfileFor returns workload-specific guidance.
func ProfileFor(workload string) WorkloadProfile {
	switch strings.ToLower(strings.TrimSpace(workload)) {
	case "couchbase":
		return WorkloadProfile{
			Name: "couchbase", ZeroDowntime: true, DocsURL: "",
			DefaultSvcs: []string{"couchbase-server"},
			Hints: []string{
				"Patch one node at a time; keep quorum.",
				"Before reboot: failover or ensure auto-failover is enabled.",
				"After start: wait for node recovery / rebalance if required.",
				"Do not patch majority of nodes concurrently.",
			},
			PreCommands: func(host string, idx, total int) []string {
				return []string{
					"echo pre-couchbase host=" + host + " idx=" + fmt.Sprintf("%d/%d", idx+1, total),
					"command -v couchbase-cli >/dev/null && echo couchbase-cli-present || echo couchbase-cli-missing",
					"systemctl is-active couchbase-server 2>/dev/null || true",
				}
			},
			PostCommands: func(host string, idx, total int) []string {
				return []string{
					"systemctl start couchbase-server 2>/dev/null || true",
					"sleep 5; systemctl is-active couchbase-server 2>/dev/null || true",
					"echo post-couchbase: verify cluster recovery/rebalance from another healthy node",
				}
			},
			HealthCheck: func(c *ssh.Client) (string, error) {
				return Run(c, "systemctl is-active couchbase-server 2>/dev/null || echo inactive")
			},
		}
	case "hashicorp_vault", "vault":
		return WorkloadProfile{
			Name: "hashicorp_vault", ZeroDowntime: false, DocsURL: "https://developer.hashicorp.com/vault",
			DefaultSvcs: []string{"vault"},
			Hints: []string{
				"Legacy HC Vault often breaks after reboot if auto-unseal is misconfigured.",
				"Prefer standby first; never reboot the active primary until standbys are healthy.",
				"After reboot: check `vault status` — sealed nodes need unseal/auto-unseal.",
				"Take a raft snapshot or ensure storage backend is healthy before patching.",
			},
			PreCommands: func(host string, idx, total int) []string {
				return []string{
					"command -v vault >/dev/null && vault status -format=json 2>/dev/null | head -c 400 || echo vault-status-unavailable",
					"systemctl is-active vault 2>/dev/null || true",
				}
			},
			PostCommands: func(host string, idx, total int) []string {
				return []string{
					"systemctl start vault 2>/dev/null || true",
					"sleep 3; command -v vault >/dev/null && vault status 2>/dev/null | head -n 20 || true",
					"echo IMPORTANT: if Sealed=true, run auto-unseal recovery or operator unseal before continuing",
				}
			},
			HealthCheck: func(c *ssh.Client) (string, error) {
				return Run(c, "vault status 2>/dev/null | head -n 15 || systemctl is-active vault")
			},
		}
	case "wslvault":
		return WorkloadProfile{
			Name: "wslvault", ZeroDowntime: true, DocsURL: WSLVaultDocs,
			DefaultSvcs: []string{"wslvault", "wslvault-api"},
			Hints: []string{
				"WSLVault is HA + multi-region with two-way sync — safer rolling patch than legacy Vault.",
				"Drain / patch one region or pod/node at a time; peers continue serving.",
				"Before reboot: confirm replication lag is healthy; after: verify sync and leases.",
				"Prefer moving pods (k8s) or stopping one region node, patch, start, wait for sync, then next.",
				"UI: https://vault-ui.workstation.co.uk/login · docs: https://www.wslvault.org/",
			},
			PreCommands: func(host string, idx, total int) []string {
				return []string{
					"echo wslvault-pre host=" + host + " — ensuring peer HA before maintenance",
					"systemctl is-active wslvault wslvault-api 2>/dev/null || true",
					"curl -sS -m 5 http://127.0.0.1:8200/v1/sys/health 2>/dev/null | head -c 300 || curl -sS -m 5 https://127.0.0.1:8200/v1/sys/health 2>/dev/null | head -c 300 || echo health-local-unavailable",
					"if command -v kubectl >/dev/null; then echo k8s-present; kubectl get pods -A 2>/dev/null | grep -i wslvault | head -n 5 || true; fi",
				}
			},
			PostCommands: func(host string, idx, total int) []string {
				return []string{
					"systemctl start wslvault wslvault-api 2>/dev/null || true",
					"sleep 5; curl -sS -m 8 http://127.0.0.1:8200/v1/sys/health 2>/dev/null | head -c 300 || true",
					"echo wslvault-post: confirm multi-region sync / replication_lag before patching next node",
				}
			},
			HealthCheck: func(c *ssh.Client) (string, error) {
				return Run(c, "curl -sS -m 5 http://127.0.0.1:8200/v1/sys/health 2>/dev/null || systemctl is-active wslvault 2>/dev/null || echo check-manual")
			},
		}
	default:
		return WorkloadProfile{
			Name: "generic", ZeroDowntime: false,
			Hints: []string{
				"Rolling order recommended even for generic hosts.",
				"Run pre/post scripts; ensure critical services restart after reboot.",
			},
			PreCommands:  func(string, int, int) []string { return nil },
			PostCommands: func(string, int, int) []string { return nil },
		}
	}
}

// BuildHeuristicPlan orders hosts and describes steps without LLM.
func BuildHeuristicPlan(workload string, hosts []HostTarget, rebootPolicy string) model.LinuxPatchPlan {
	p := ProfileFor(workload)
	order := make([]string, 0, len(hosts))
	for _, h := range hosts {
		order = append(order, h.Raw)
	}
	steps := []string{
		"Inventory each host (OS flavor + pending packages) over SSH",
		"Apply workload pre-checks: " + p.Name,
		"Patch one host at a time (rolling) to limit blast radius",
		"Run operator pre_script then package upgrade",
		"Reboot per policy (" + rebootPolicy + ") if required",
		"Ensure systemd services + workload post-checks",
		"Run operator post_script; proceed to next host only if healthy",
	}
	if p.ZeroDowntime {
		steps = append([]string{"Zero-downtime rolling: keep peer capacity online while one node is in maintenance"}, steps...)
	}
	return model.LinuxPatchPlan{
		Strategy:      p.Name + " rolling SSH patch",
		ZeroDowntime:  p.ZeroDowntime,
		Order:         order,
		Steps:         steps,
		WorkloadHints: p.Hints,
		DocsURL:       p.DocsURL,
		Warnings:      workloadWarnings(p),
	}
}

func workloadWarnings(p WorkloadProfile) []string {
	var w []string
	if p.Name == "hashicorp_vault" {
		w = append(w, "HC Vault may remain sealed after reboot — verify auto-unseal before continuing the roll.")
	}
	if p.Name == "wslvault" {
		w = append(w, "WSLVault HA allows safer rolls; still wait for sync before the next region/node.")
	}
	if p.Name == "couchbase" {
		w = append(w, "Confirm auto-failover / rebalance capacity before rebooting a Couchbase node.")
	}
	return w
}
