package database

import (
	"testing"
	"time"
)

// The case that actually breaks production: a server booted in the last quarter
// of the year has to keep partitioning into the next one.
func TestUsagePartitions_RollsIntoNextYear(t *testing.T) {
	now := time.Date(2026, time.December, 14, 9, 30, 0, 0, time.UTC)

	got := usagePartitions(now, 3)

	want := []string{
		"usage_records_2026_12",
		"usage_records_2027_01",
		"usage_records_2027_02",
		"usage_records_2027_03",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d partitions, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("partition %d = %q, want %q", i, got[i].Name, w)
		}
	}
}

// Ranges must be contiguous and half-open, or a row lands between two
// partitions and the insert fails exactly as if no partition existed.
func TestUsagePartitions_RangesAreContiguous(t *testing.T) {
	now := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)

	got := usagePartitions(now, 3)

	if !got[0].From.Equal(time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("first partition starts at %s, want the 1st of the month", got[0].From)
	}
	for i := 1; i < len(got); i++ {
		if !got[i].From.Equal(got[i-1].To) {
			t.Errorf("gap between %s (ends %s) and %s (starts %s)",
				got[i-1].Name, got[i-1].To, got[i].Name, got[i].From)
		}
	}
}

// The horizon has to cover the month it is asked about, whatever day it is run.
func TestUsagePartitions_CoversLastDayOfMonth(t *testing.T) {
	now := time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC)

	got := usagePartitions(now, 3)

	if got[0].Name != "usage_records_2026_12" {
		t.Fatalf("first partition = %q, want usage_records_2026_12", got[0].Name)
	}
	if !now.Before(got[0].To) {
		t.Errorf("partition %s ends at %s, which does not cover %s",
			got[0].Name, got[0].To, now)
	}
}
