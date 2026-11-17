package config

import (
	"strings"
	"testing"
)

func TestUCIRevisionIgnoresAnonymousIDsAndDetectsChanges(t *testing.T) {
	first := `{"values":{
		"main":{".type":"settings",".index":0,".anonymous":false,"enabled":"1","wan_device":["wan"]},
		"cfg012345":{".type":"service",".index":1,".anonymous":true,"label":"Example","domain":["a.example"]}
	}}`
	baseline, before, err := ParseRPC([]byte(first))
	if err != nil {
		t.Fatal(err)
	}
	renamed := strings.ReplaceAll(first, "cfg012345", "cfg023456")
	cfg, after, err := ParseRPC([]byte(renamed))
	if err != nil || before != after || baseline.Services[0].ID == cfg.Services[0].ID {
		t.Fatalf("unexpected revision after anonymous ID changed: %s, %v", after, err)
	}
	_, after, err = ParseRPC([]byte(strings.ReplaceAll(renamed, "a.example", "b.example")))
	if err != nil || before == after {
		t.Fatalf("revision did not change with settings: %s, %v", after, err)
	}
}
