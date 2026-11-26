package main

import "testing"

func TestClientRoutesDoNotAcceptArbitraryTargets(t *testing.T) {
	id := "12345678-1234-1234-1234-123456789abc"
	if got := clientRoute(0, id); got != "codex://threads/"+id {
		t.Fatal(got)
	}
	if got := clientRoute(0, "../../bad"); got != "codex://" {
		t.Fatal(got)
	}
	if clientRoute(1, "") != "antigravity://" || clientRoute(2, "") != "antigravity://" || clientRoute(3, "") != "https://chat.mistral.ai/code" {
		t.Fatal("provider route changed")
	}
	if noticeCopy(false) == noticeCopy(true) {
		t.Fatal("kinds must be distinguishable")
	}
}
