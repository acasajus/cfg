package cfg

import (
	"fmt"
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

/* Start tests */

func TestSplitPath(t *testing.T) {
	expected := []string{"a", "b", "c"}
	for _, source := range []string{"a/b/c", "/a/b/c", "//a/b/c", "a/b/c/", "////a////b//c/////"} {
		res := SplitPath(source)
		if !equalSlices(res, expected) {
			t.Fatal(fmt.Sprintf("Unexpected split: %v expected %v", res, expected))
		}
	}
}

func TestLoadErrors(t *testing.T) {
	data := "a = 1\na {\nop = 1\n}"
	expected := "Section a defined under / is already defined (line 2)"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error: ", err)
	}
	data = "a { crap\n}"
	expected = "Expected inheriting section defined with '< section_name' but 'crap' found (line 1)"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
	data = "a=1\na=1"
	expected = "a already exists (line 2)"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
	data = "a=1\nb+=1"
	expected = "Option b was not previously defined (line 2)"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
	data = "s{\na=1\nb+=1\n"
	expected = "Option b was not previously defined (line 3)"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
	data = "s{\n}\ns2{<a\n}"
	expected = "Inheritance section a for section s2 does not exist"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
	data = "s1{<s3\n}\ns2{<s1\n}\ns3{<s2\n}"
	expected = "Circular inheritance loop found: s3 < s2 < s1 < s3"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
	data = "s1 {\n}\ns2 {<s1\ns21{<s2/s21/s211\ns211{<s2\n}\n}\n}\n"
	expected = "Cannot inherit from a direct parent to prevent recursive loops (s2 is parent of s2/s21/s211)"
	if _, err := NewCFGFromString(data); err == nil || err.Error() != expected {
		t.Error("Didn't receive expected error:", err)
	}
}

func TestLoadDump(t *testing.T) {
	var err error
	var cfg, cfg2 *CFG
	data := "s1 {< s3\n}\ns2 {\n\ts21 {< s1\n\t\top1 = a\n\t\top1 += b\n\t}\n}\n#Stupid comment\ns3 {< s2\n}\n"
	cfg, err = NewCFGFromString(data)
	if err != nil {
		t.Error("Error wile loading CFG: " + err.Error())
	}
	out := cfg.String()
	if out != data {
		t.Error("Re dump differs from original dump")
	}
	cfg2, err = NewCFGFromString(data)
	if err != nil {
		t.Error("Error wile loading CFG: " + err.Error())
	}
	if out != cfg2.String() {
		t.Error("Re dump differs from original dump")
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

func TestFromFile(t *testing.T) {
	_, err := NewCFGFromFile("nonexistantfile")
	if err == nil {
		t.Error("Didn't complain when opening a non existant file")
	}
	if _, err := NewCFGFromFile("examples/simple.cfg"); err != nil {
		t.Error(err)
	}
}

func TestCloneEqual(t *testing.T) {
	data := "s1 {\nop1 = val1\nop1 += val1a\n}\ns2 {<s1\ns21{\nop211=val211\n}\ns22{\n}\n}\nop1=a"
	cfg, err := NewCFGFromString(data)
	if err != nil {
		t.Error(err)
	}
	dup, err := cfg.Clone()
	if err != nil {
		t.Error(err)
	}
	if !dup.Equal(cfg) {
		t.Error("Not equal!")
	}
}

func TestExists(t *testing.T) {
	data := "s1 {\nop1 = val1\nop1 += val1a\n}\ns2 {<s1\ns21{\nop211=val211\n}\ns22{\n}\n}\nop1=a"
	cfg, err := NewCFGFromString(data)
	if err != nil {
		t.Error("Error wile loading CFG: " + err.Error())
	}
	if !cfg.Exists("s1") {
		t.Error("Section doesn't exist")
	}
	if cfg.ExistsOption("s1") {
		t.Error("Section exists as option")
	}
	if !cfg.ExistsSection("s1") {
		t.Error("Section doesn't exist as section")
	}
	if !cfg.Exists("s2/s21") {
		t.Error("Section doesn't exist")
	}
	if cfg.Exists("s2/s31") {
		t.Error("Section exists")
	}
	if cfg.Exists("s0") {
		t.Error("Section exists")
	}
	if !cfg.Exists("s1/op1") {
		t.Error("Option doesn't exist")
	}
	if !cfg.Exists("s2/op1") {
		t.Error("Option doesn't exist")
	}
	if !cfg.ExistsOption("op1") {
		t.Error("Option doesn't exist")
	}
}

func TestInsertContents(t *testing.T) {
	data1 := "s2 {\ns21{\nop211=a\n}\ns22{\n}\n}\ns3{<s2\nop3=b\n}"
	data2 := "s1 {\nop1 = val1\nop1 += val1a\n}\ns2 {<s1\ns21{\nop211=val211\n}\ns22{\n}\n}\nop1=a"
	var cfg, in_cfg *CFG
	var err error
	cfg, err = NewCFGFromString(data1)
	if err != nil {
		t.Error(err)
	}
	in_cfg, err = NewCFGFromString(data2)
	if err != nil {
		t.Error(err)
	}
	if err = cfg.InsertContents(in_cfg); err != nil {
		t.Error(err)
	}
	expected := "s2 {\n\ts21 {\n\t\top211 = val211\n\t}\n\ts22 {\n\t}\n\top1 = val1\n\top1 += val1a\n}\ns3 {< s2\n\top3 = b\n}\nop1 = a\ns1 {\n\top1 = val1\n\top1 += val1a\n}\n"
	if expected_cfg, err := NewCFGFromString(expected); err != nil {
		t.Error(err)
	} else {
		if !expected_cfg.Equal(cfg) {
			t.Error("Merge didn't go as expected")
		}
	}
}
