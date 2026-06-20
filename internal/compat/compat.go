package compat

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const DefaultTestURL = "https://www.gstatic.com/generate_204"

var selectableTypes = map[string]bool{
	"selector": true,
	"urltest":  true,
	"fallback": true,
}

type Capabilities struct {
	Version bool
	Configs bool
	Proxies bool
	Delay   bool
}

type Version struct {
	Core string
	Meta string
}

type Config struct {
	Mode string
}

type DelayHistory struct {
	Time  time.Time `json:"time"`
	Delay int       `json:"delay"`
}

type Proxy struct {
	Name    string
	Type    string
	Now     string
	All     []string
	Alive   bool
	Hidden  bool
	TestURL string
	History []DelayHistory
}

type ProxyGroup struct {
	Name    string
	Type    string
	Now     string
	Options []string
	Alive   bool
	Hidden  bool
	TestURL string
}

type Snapshot struct {
	Version Version
	Config  Config
	Proxies map[string]Proxy
	Groups  []ProxyGroup
}

func NormalizeVersion(raw map[string]any) Version {
	return Version{
		Core: stringValue(raw["version"]),
		Meta: stringValue(raw["meta"]),
	}
}

func NormalizeConfig(raw map[string]any) Config {
	return Config{
		Mode: strings.ToLower(stringValue(raw["mode"])),
	}
}

func NormalizeProxies(raw map[string]any) (map[string]Proxy, []ProxyGroup, error) {
	proxyRoot, ok := raw["proxies"]
	if !ok {
		return nil, nil, fmt.Errorf("missing proxies field")
	}

	items, ok := proxyRoot.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("invalid proxies field")
	}

	proxies := make(map[string]Proxy, len(items))
	groups := make([]ProxyGroup, 0)
	for name, value := range items {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		proxy := normalizeProxy(name, entry)
		proxies[name] = proxy
		if selectableTypes[strings.ToLower(proxy.Type)] {
			groups = append(groups, ProxyGroup{
				Name:    proxy.Name,
				Type:    proxy.Type,
				Now:     proxy.Now,
				Options: append([]string(nil), proxy.All...),
				Alive:   proxy.Alive,
				Hidden:  proxy.Hidden,
				TestURL: proxy.TestURL,
			})
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return proxies, groups, nil
}

func NormalizeProxy(raw map[string]any) Proxy {
	return normalizeProxy(stringValue(raw["name"]), raw)
}

func normalizeProxy(name string, raw map[string]any) Proxy {
	all := stringSlice(raw["all"])
	history := delayHistory(raw["history"])
	return Proxy{
		Name:    fallbackString(name, stringValue(raw["name"])),
		Type:    strings.ToLower(stringValue(raw["type"])),
		Now:     stringValue(raw["now"]),
		All:     all,
		Alive:   boolValue(raw["alive"]),
		Hidden:  boolValue(raw["hidden"]),
		TestURL: fallbackString(stringValue(raw["testUrl"]), DefaultTestURL),
		History: history,
	}
}

func delayHistory(raw any) []DelayHistory {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]DelayHistory, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, DelayHistory{
			Time:  timeValue(entry["time"]),
			Delay: int(numberValue(entry["delay"])),
		})
	}
	return out
}

func stringSlice(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value := stringValue(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func stringValue(raw any) string {
	if value, ok := raw.(string); ok {
		return value
	}
	return ""
}

func boolValue(raw any) bool {
	value, ok := raw.(bool)
	return ok && value
}

func numberValue(raw any) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func timeValue(raw any) time.Time {
	if text, ok := raw.(string); ok {
		parsed, _ := time.Parse(time.RFC3339, text)
		return parsed
	}
	return time.Time{}
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
