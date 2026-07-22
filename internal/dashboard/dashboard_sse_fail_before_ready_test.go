package dashboard_test

import (
	"os/exec"
	"testing"
)

func TestDashboardSSEFailsDefinitivelyBeforeReadyWithoutLoops(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required for dashboard JavaScript behavior test")
	}
	html := renderDashboardForTest(t)
	valid := extractJSFunction(t, html, "validStreamCursor")
	reset := extractJSFunction(t, html, "resetStreamCursor")
	advance := extractJSFunction(t, html, "advanceStreamCursor")
	appendLine := extractJSFunction(t, html, "appendTimeline")
	current := extractJSFunction(t, html, "streamIsCurrent")
	failProtocol := extractJSFunction(t, html, "failStreamProtocol")
	connect := extractJSFunction(t, html, "connectStream")
	schedule := extractJSFunction(t, html, "scheduleStreamReconnect")

	script := `
const maxUint64Decimal = "18446744073709551615";
const elements = {
  afterSeq: {value: "10"},
  eventKind: {value: ""},
  eventNamespace: {value: ""},
  eventRequestId: {value: ""},
  timeline: {textContent: "", dataset: {empty: "1"}, scrollTop: 0, scrollHeight: 0},
  streamBadge: {textContent: "", className: ""}
};
const el = (id) => {
  if (elements[id]) return elements[id];
  throw new Error("unexpected element " + id);
};

let lastSeq = "10";
let after = "10";
let streamGeneration = 0;
let es = null;
let streamRetryAttempt = 0;
let streamRetryTimer = null;
let retrying = false;
let apiPrefix = "/api";
let currentMission = "m1";
let inspectBase = "/api";

// Overwrite window setTimeout to trap timers synchronously
let timers = [];
const window = {
  setTimeout: (cb, ms) => {
    timers.push({cb, ms});
    return timers.length;
  },
  clearTimeout: (id) => {
    timers = timers.filter((_, i) => i !== id - 1);
  }
};
const setTimeout = window.setTimeout;
const clearTimeout = window.clearTimeout;

` + valid + "\n" + reset + "\n" + advance + "\n" + appendLine + "\n" + current + "\n" + failProtocol + "\n" + schedule + `

class MockEventSource {
  constructor(url) {
    this.url = url;
    this.listeners = {};
    this.onerror = null;
    this.closed = false;
  }
  addEventListener(name, cb) {
    this.listeners[name] = this.listeners[name] || [];
    this.listeners[name].push(cb);
  }
  close() {
    this.closed = true;
  }
  dispatch(name, data, lastEventId) {
    if (!this.listeners[name]) return;
    this.listeners[name].forEach(cb => cb({data: data, lastEventId: lastEventId}));
  }
}
const EventSource = MockEventSource;

` + connect + `

// 1. Initial connection
connectStream();
const source = es;
if (!source) throw new Error("EventSource not created");

// 2. Simulate transport error before ready
if (!source.onerror) throw new Error("onerror handler not registered");
source.onerror({ type: "error" });

// 3. Verify that the protocol was failed and NO retry was scheduled
if (!elements.timeline.textContent.includes("falha de transporte antes do handshake ready")) {
	throw new Error("timeline missing evidence of failure before ready: " + elements.timeline.textContent);
}

if (timers.length > 0) {
	throw new Error("a retry was scheduled before ready was seen: " + timers.length + " timers");
}

if (streamRetryAttempt !== 0) {
	throw new Error("streamRetryAttempt incremented before ready: " + streamRetryAttempt);
}

// 4. Verify stream was fenced
if (es !== null) {
	throw new Error("es variable was not set to null upon failure");
}
`

	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("dashboard failure before ready behavior failed: %v\n%s", err, output)
	}
}
