package dashboard_test

import (
	"strings"
	"testing"
)

func TestDashboardPayloadLimitJavascript(t *testing.T) {
	js := renderDashboardForTest(t)

	if !strings.Contains(js, "MAX_PAYLOAD_SIZE = 512 * 1024") {
		t.Errorf("expected string limit condition on max payload size constant")
	}
	if !strings.Contains(js, "event com payload JSON malformado e que excede limite (512KB)") {
		t.Errorf("expected string limit condition on event handler")
	}
	if !strings.Contains(js, "page com payload JSON que excede limite (512KB)") {
		t.Errorf("expected string limit condition on page handler")
	}
	if !strings.Contains(js, "ready com payload JSON que excede limite (512KB)") {
		t.Errorf("expected string limit condition on ready handler")
	}
	if !strings.Contains(js, "terminal_error com payload JSON que excede limite (512KB)") {
		t.Errorf("expected string limit condition on terminal_error handler")
	}
}
