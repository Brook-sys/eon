package inspect

import "testing"

func TestSSEDrainPacerBoundsImmediatePagesAndResets(t *testing.T) {
	var pacer sseDrainPacer
	for page := 1; page < maxSSEImmediatePages; page++ {
		if !pacer.continueImmediately(true) {
			t.Fatalf("page %d yielded before burst limit %d", page, maxSSEImmediatePages)
		}
	}
	if pacer.continueImmediately(true) {
		t.Fatalf("page %d did not force timer yield", maxSSEImmediatePages)
	}
	if !pacer.continueImmediately(true) {
		t.Fatal("new burst did not resume immediate draining after forced yield")
	}
	if pacer.continueImmediately(false) {
		t.Fatal("finite backlog completion requested immediate continuation")
	}
	if !pacer.continueImmediately(true) {
		t.Fatal("backlog completion did not reset the next burst")
	}
}
