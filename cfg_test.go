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
	for _, val := range []string{"val1", "val2"} {
		if err := cfg.SetOption("/op1", val, ""); err != nil {
			t.Error(err)
		}
		if rval, ok := cfg.GetOption("op1"); !ok || val != rval {
			t.Error("Could not retrieve op1 or val was different than expected (" + rval + " vs " + val + ")")
		}
	}
	if err := cfg.SetOption("nop/as", "ASD", ""); err == nil {
		t.Error("Allowed to set an option with inexistant parent section")
	}
	if err := cfg.SetOption("", "ASD", ""); err == nil {
		t.Error("Allowed to set an option with inexistant name")
	}
	if d := cfg.GetValue("", "DEF"); d != "DEF" {
		t.Error("Didn't get default value")
	}
	if d := cfg.GetValueArray("", []string{"DEF"}); len(d) != 1 || d[0] != "DEF" {
		t.Error("Didn't get default value")
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
