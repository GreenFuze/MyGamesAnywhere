package http

import "testing"

func TestConfigJSONObjectDeepEqual(t *testing.T) {
	if !configJSONObjectDeepEqual(`{"a":1,"b":{"c":2}}`, `{"b":{"c":2},"a":1}`) {
		t.Fatal("expected equal (key order independent)")
	}
	if !configJSONObjectDeepEqual("", "{}") {
		t.Fatal("expected empty to match {}")
	}
	if configJSONObjectDeepEqual(`{"a":1}`, `{"a":2}`) {
		t.Fatal("expected unequal values")
	}
	if configJSONObjectDeepEqual(`{"a":1}`, `{"a":1,"b":2}`) {
		t.Fatal("expected unequal keys")
	}
	if configJSONObjectDeepEqual(`not json`, `{}`) {
		t.Fatal("invalid JSON should not equal")
	}
}
