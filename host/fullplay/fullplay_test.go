package fullplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogIDsFrozen(t *testing.T) {
	have := map[string]bool{}
	for _, s := range Catalog() {
		if s.ID == "" {
			t.Fatal("catalog entry missing ID")
		}
		if have[s.ID] {
			t.Fatalf("duplicate scenario id %q — append a new id, do not reuse", s.ID)
		}
		have[s.ID] = true
	}
	for _, id := range frozenIDs {
		if !have[id] {
			t.Fatalf("frozen scenario %q was removed from Catalog — add it back or append a replacement with a new id", id)
		}
	}
}

func TestCatalogCoversEveryPluginFolder(t *testing.T) {
	covered := map[string]bool{}
	for _, s := range Catalog() {
		covered[s.PluginID] = true
	}
	for _, id := range bundledPluginIDs(t) {
		if !covered[id] {
			t.Errorf("plugin %q has no fullplay scenario — append one to Catalog() and frozenIDs", id)
		}
	}
}

func TestCatalogUIContracts(t *testing.T) {
	for _, s := range Catalog() {
		s := s
		t.Run(s.ID, func(t *testing.T) {
			for _, chk := range s.UI {
				body := readRepoFile(t, chk.Rel...)
				for _, want := range chk.Has {
					if !strings.Contains(body, want) {
						t.Errorf("missing %q in %s — %s", want, filepath.Join(chk.Rel...), chk.Why)
					}
				}
				for _, bad := range chk.HasNot {
					if strings.Contains(body, bad) {
						t.Errorf("found forbidden %q in %s — %s", bad, filepath.Join(chk.Rel...), chk.Why)
					}
				}
			}
		})
	}
}

func TestCatalogPlaysFullGames(t *testing.T) {
	if _, err := os.Stat(pluginsRoot(t)); err != nil {
		t.Fatal(err)
	}
	for _, s := range Catalog() {
		s := s
		t.Run(s.ID, func(t *testing.T) {
			if s.Play == nil {
				t.Fatal("scenario has no Play func")
			}
			h := openHost(t, s.PluginID, s.NumRanges, s.TotalShots)
			s.Play(t, h)
			if s.Finished != "" {
				h.requirePhase(s.PhasePath, s.Finished)
			}
		})
	}
}
