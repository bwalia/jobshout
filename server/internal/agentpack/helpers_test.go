package agentpack

import (
	"encoding/json"
	"testing"
	"time"
)

func jsonForTest(pkg *Package) ([]byte, error) {
	return json.Marshal(pkg)
}

func parseDay(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
}
