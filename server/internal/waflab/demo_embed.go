package waflab

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed demo/waf_rules/*.json demo/waf_policies/*.json
var demoFS embed.FS

func loadDemoMaps(kind string) ([]map[string]any, error) {
	dir := path.Join("demo", kind)
	entries, err := fs.ReadDir(demoFS, dir)
	if err != nil {
		return nil, fmt.Errorf("demo %s: %w", kind, err)
	}
	var out []map[string]any
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := demoFS.ReadFile(path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no demo payloads for %s", kind)
	}
	return out, nil
}
