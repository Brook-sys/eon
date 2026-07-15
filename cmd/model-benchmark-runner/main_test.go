package main

import "testing"

func TestParseContexts(t *testing.T) {
	got, err := parseContexts("2048, 4096,8192")
	if err != nil || len(got) != 3 || got[1] != 4096 {
		t.Fatalf("got=%v err=%v", got, err)
	}
	for _, value := range []string{"", "2048,2048", "0", "bad"} {
		if _, err := parseContexts(value); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}
