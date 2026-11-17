package manifest

import (
	"errors"
	"math/big"
	"strings"
)

// CompareVersion compares canonical three-part numeric versions without overflow.
func CompareVersion(a, b string) int {
	if len(strings.Split(a, ".")) != 3 || len(strings.Split(b, ".")) != 3 {
		return -1
	}
	for i, x := range strings.Split(a, ".") {
		parts := strings.Split(b, ".")
		if len(parts) != 3 {
			return -1
		}
		aa, ok := new(big.Int).SetString(x, 10)
		bb, ok2 := new(big.Int).SetString(parts[i], 10)
		if !ok || !ok2 {
			return -1
		}
		if c := aa.Cmp(bb); c != 0 {
			return c
		}
	}
	return 0
}
func (m Manifest) validateDependencies() error {
	if len(m.Dependencies) > 8 || len(m.Contributes.EntryPoints) > 8 {
		return errors.New("依赖或公开入口数量超过限制")
	}
	seen := map[string]bool{}
	targets := map[string]bool{}
	for _, d := range m.Dependencies {
		if !commandPattern.MatchString(d.ID) || !ValidID(d.PluginID) || d.PluginID == m.ID || seen[d.ID] || targets[d.PluginID] || !versionPattern.MatchString(d.MinVersion) || len(d.MinVersion) > 32 || len(d.Entries) == 0 || len(d.Entries) > 8 {
			return errors.New("依赖声明无效或重复")
		}
		seen[d.ID] = true
		targets[d.PluginID] = true
		entries := map[string]bool{}
		for _, e := range d.Entries {
			if !commandPattern.MatchString(e.ID) || e.Version != "1" || entries[e.ID] {
				return errors.New("依赖入口无效或重复")
			}
			entries[e.ID] = true
		}
	}
	for _, v := range m.Contributes.UIViews {
		if len(v.Dependencies) > 8 || len(v.Dependencies) > 0 && v.BridgeVersion != "2" {
			return errors.New("视图依赖需要桥 2")
		}
		used := map[string]bool{}
		for _, id := range v.Dependencies {
			if !seen[id] || used[id] {
				return errors.New("视图引用未知或重复依赖")
			}
			used[id] = true
		}
	}
	exports := map[string]bool{}
	for _, e := range m.Contributes.EntryPoints {
		if !commandPattern.MatchString(e.ID) || e.Version != "1" || e.Input != "image" || exports[e.ID] {
			return errors.New("公开图片入口无效")
		}
		exports[e.ID] = true
		found := false
		for _, c := range m.Contributes.Commands {
			if c.ID != e.CommandID || c.UIView == "" || c.StartHidden || c.SearchOnly {
				continue
			}
			for _, v := range m.Contributes.UIViews {
				if v.ID != c.UIView || v.BridgeVersion != "2" {
					continue
				}
				for _, rule := range c.InputMatchers {
					if rule.Kind == "image" {
						found = true
					}
				}
			}
		}
		if !found {
			return errors.New("公开入口必须关联接受图片的桥 2 普通视图命令")
		}
	}
	return nil
}
