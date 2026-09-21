package linuxpatch

import (
	"testing"
)

func TestParseHosts(t *testing.T) {
	hs := ParseHosts("root@a.example:2222, b.example\nuser@c.example", "ubuntu")
	if len(hs) != 3 {
		t.Fatalf("got %d %#v", len(hs), hs)
	}
	if hs[0].Port != "2222" || hs[0].User != "root" {
		t.Fatalf("%#v", hs[0])
	}
	if hs[1].User != "ubuntu" || hs[1].Host != "b.example" {
		t.Fatalf("%#v", hs[1])
	}
}

func TestBuildHeuristicPlan_WSLVault(t *testing.T) {
	p := BuildHeuristicPlan("wslvault", ParseHosts("n1,n2", "root"), "if_needed")
	if !p.ZeroDowntime {
		t.Fatal("expected ZDT")
	}
	if p.DocsURL == "" {
		t.Fatal("expected docs")
	}
	if len(p.Order) != 2 {
		t.Fatal(p.Order)
	}
}

func TestProfileVaultWarnings(t *testing.T) {
	p := BuildHeuristicPlan("hashicorp_vault", ParseHosts("v1", "root"), "never")
	if p.ZeroDowntime {
		t.Fatal("HC vault should not claim ZDT by default")
	}
	if len(p.Warnings) == 0 {
		t.Fatal("expected seal warning")
	}
}

func TestParseServices(t *testing.T) {
	if len(ParseServices("nginx, vault;redis")) != 3 {
		t.Fatal("services")
	}
}
