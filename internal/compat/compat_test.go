package compat

import "testing"

func TestNormalizeProxiesMihomoShape(t *testing.T) {
	t.Parallel()

	raw := map[string]any{
		"proxies": map[string]any{
			"GLOBAL": map[string]any{
				"type":    "Selector",
				"now":     "NodeA",
				"all":     []any{"NodeA", "NodeB"},
				"alive":   true,
				"testUrl": "https://example.com",
			},
			"NodeA": map[string]any{
				"type":    "Shadowsocks",
				"alive":   true,
				"history": []any{map[string]any{"delay": float64(30)}},
			},
		},
	}

	proxies, groups, err := NormalizeProxies(raw)
	if err != nil {
		t.Fatalf("NormalizeProxies failed: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "GLOBAL" {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if proxies["NodeA"].History[0].Delay != 30 {
		t.Fatalf("unexpected history: %#v", proxies["NodeA"].History)
	}
}

func TestNormalizeProxiesClashCompatibleShape(t *testing.T) {
	t.Parallel()

	raw := map[string]any{
		"proxies": map[string]any{
			"Proxy": map[string]any{
				"name": "Proxy",
				"type": "URLTest",
				"now":  "NodeB",
				"all":  []any{"NodeA", "NodeB"},
			},
			"NodeB": map[string]any{
				"name": "NodeB",
				"type": "Trojan",
			},
		},
	}

	proxies, groups, err := NormalizeProxies(raw)
	if err != nil {
		t.Fatalf("NormalizeProxies failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("unexpected groups length: %d", len(groups))
	}
	if groups[0].Type != "urltest" {
		t.Fatalf("unexpected group type: %s", groups[0].Type)
	}
	if proxies["NodeB"].TestURL != DefaultTestURL {
		t.Fatalf("expected default test url, got %q", proxies["NodeB"].TestURL)
	}
}
