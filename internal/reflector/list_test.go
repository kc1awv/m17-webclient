package reflector

import "testing"

func TestFetchModulesReturnsCopy(t *testing.T) {
	ls := NewListStore()
	ls.moduleMu.Lock()
	ls.moduleCache["m17-test"] = cachedModules{Modules: []string{"A", "B"}}
	ls.moduleMu.Unlock()

	mods := ls.FetchModules("m17-test")
	if len(mods) != 2 || mods[0] != "A" || mods[1] != "B" {
		t.Fatalf("unexpected modules %v", mods)
	}

	mods[0] = "Z"

	ls.moduleMu.RLock()
	first := ls.moduleCache["m17-test"].Modules[0]
	ls.moduleMu.RUnlock()
	if first != "A" {
		t.Fatalf("internal module list modified: %v", ls.moduleCache["m17-test"].Modules)
	}
}

func TestGetReflectorsReturnsCopy(t *testing.T) {
	ls := NewListStore()
	ls.mu.Lock()
	ls.reflectorList = []ReflectorInfo{{Designator: "M17-AAA", Name: "Test", Address: "1.2.3.4:17000", Slug: "m17-aaa"}}
	ls.mu.Unlock()

	list := ls.GetReflectors()
	if len(list) != 1 {
		t.Fatalf("unexpected reflector list %v", list)
	}

	list[0].Name = "Changed"

	ls.mu.RLock()
	name := ls.reflectorList[0].Name
	ls.mu.RUnlock()
	if name != "Test" {
		t.Fatalf("internal reflector list modified: %v", ls.reflectorList)
	}
}
