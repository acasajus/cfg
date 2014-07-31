package cfg

import (
	"fmt"
	"os"
	"testing"
)

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for iP, iS := range a {
		if b[iP] != iS {
			return false
		}
	}
	return true
}

func TestSplitPath(t *testing.T) {
	expected := []string{"a", "b", "c"}
	for _, source := range []string{"a/b/c", "/a/b/c", "//a/b/c", "a/b/c/", "////a////b//c/////"} {
		res := SplitPath(source)
		if !equalSlices(res, expected) {
			t.Fatal(fmt.Sprintf("Unexpected split: %v expected %v", res, expected))
		}
	}
}

func TestSetOptionString(t *testing.T) {
	cfg := NewCFG()
	if err := cfg.SetOption("op1", "val1", ""); err != nil {
		t.Error(err)
	}
	if val, ok := cfg.GetOption("op1"); !ok || val != "val1" {
		t.Error("Could not retrieve op1 or val was different than expected (" + val + ")")
	}
}

func TestLoadFromReader(t *testing.T) {
	fi, err := os.Open("example.cfg")
	if err != nil {
		panic(err)
	}
	defer fi.Close()
	cfg, err := NewCFGFromReader(fi)
	if err != nil {
		t.Error(err)
	}
	cfg.Root()
}
