package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseRangeList(t *testing.T) {
	got := ParseRangeList(" 5, 2;2  5,abc,0,-1, 3")
	want := []int{2, 3, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if ParseRangeList("") != nil {
		t.Fatal("empty should be nil")
	}
}

func TestRuntimeRoundTripAndPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.xml")
	if _, err := LoadRuntime(path); err != nil {
		t.Fatal(err)
	}
	rt := &Runtime{}
	rt.SetInactiveList([]int{6, 2, 9, 2}, 6)
	if rt.InactiveRanges != "2,6" {
		t.Fatalf("set = %q", rt.InactiveRanges)
	}
	if !rt.IsInactive(2) || rt.IsInactive(3) || rt.IsInactive(9) {
		t.Fatalf("inactive map %q", rt.InactiveRanges)
	}
	if err := SaveRuntime(path, rt); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRuntime(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.InactiveList(6), []int{2, 6}) {
		t.Fatalf("reload = %v", got.InactiveList(6))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
