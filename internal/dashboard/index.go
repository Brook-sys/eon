package dashboard

import (
	"html"
	"strings"
)

func renderIndex(apiBase, defaultMissionID string) string {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = "/api"
	}
	mission := html.EscapeString(strings.TrimSpace(defaultMissionID))
	// Inline single-page experimental UI. No build step; stdlib only.
	// Secrets must never be rendered here; the page only talks to Control API.
	return `<!DOCTYPE html>
<html lang="pt-BR">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>motor-autonomo — operator dashboard</title>
<style>
:root {
  --bg: #0f1419;
  --panel: #1a2332;
  --border: #2d3a4d;
  --text: #e7ecf3;
  --muted: #8b9bb4;
  --accent: #5b9fd4;
  --ok: #3d9a6a;
  --warn: #c9a227;
  --err: #c45c5c;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  --sans: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--bg); color: var(--text);
  font: 14px/1.45 var(--sans);
}
header {
  display: flex; flex-wrap: wrap; gap: 12px; align-items: center;
  padding: 12px 16px; border-bottom: 1px solid var(--border);
  background: #121a24; position: sticky; top: 0; z-index: 2;
}
header h1 { font-size: 15px; margin: 0; font-weight: 600; letter-spacing: 0.02em; }
header .meta { color: var(--muted); font-size: 12px; }
.badge {
  display: inline-block; padding: 2px 8px; border-radius: 999px;
  border: 1px solid var(--border); font-size: 11px; color: var(--muted);
}
.badge.live { border-color: var(--ok); color: var(--ok); }
.badge.err { border-color: var(--err); color: var(--err); }
main {
  display: grid; gap: 12px; padding: 12px;
  grid-template-columns: 1fr;
}
@media (min-width: 1100px) {
  main { grid-template-columns: 1.1fr 1fr; align-items: start; }
}
section {
  background: var(--panel); border: 1px solid var(--border);
  border-radius: 10px; padding: 12px 14px;
}
section h2 {
  margin: 0 0 10px; font-size: 13px; text-transform: uppercase;
  letter-spacing: 0.06em; color: var(--muted); font-weight: 600;
}
.row { display: flex; flex-wrap: wrap; gap: 8px; align-items: end; margin-bottom: 10px; }
label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
input, select, textarea, button {
  font: inherit; color: var(--text); background: #0d1218;
  border: 1px solid var(--border); border-radius: 6px; padding: 7px 9px;
}
input, select, textarea { min-width: 160px; }
textarea { width: 100%; min-height: 64px; font-family: var(--mono); font-size: 12px; }
button {
  cursor: pointer; background: #243247; border-color: #3a4f6a;
}
button:hover { border-color: var(--accent); }
button.primary { background: #1e4d73; border-color: var(--accent); }
button:disabled { opacity: 0.5; cursor: not-allowed; }
.grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.kv { display: grid; grid-template-columns: 140px 1fr; gap: 4px 10px; font-size: 13px; }
.kv dt { color: var(--muted); }
.kv dd { margin: 0; word-break: break-word; font-family: var(--mono); font-size: 12px; }
.list { display: flex; flex-direction: column; gap: 8px; max-height: 360px; overflow: auto; }
.card {
  border: 1px solid var(--border); border-radius: 8px; padding: 10px;
  background: #121925;
}
.card h3 { margin: 0 0 6px; font-size: 13px; }
.card .id { font-family: var(--mono); font-size: 11px; color: var(--muted); }
.ops { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.timeline {
  font-family: var(--mono); font-size: 11px; max-height: 420px;
  overflow: auto; white-space: pre-wrap; background: #0d1218;
  border: 1px solid var(--border); border-radius: 8px; padding: 8px;
}
.errbox { color: var(--err); font-size: 12px; margin-top: 8px; white-space: pre-wrap; }
.okbox { color: var(--ok); font-size: 12px; margin-top: 8px; }
.muted { color: var(--muted); }
.status-RUNNING, .status-ACTIVE { color: var(--ok); }
.status-PAUSED { color: var(--warn); }
.status-STOPPED, .status-FAILED { color: var(--err); }
</style>
</head>
<body>
<header>
  <h1>motor-autonomo</h1>
  <span class="badge" id="streamBadge">SSE idle</span>
  <span class="meta" id="headerMeta">experimental control surface</span>
  <span class="meta" id="clockMeta"></span>
</header>
<main>
  <div>
    <section>
      <h2>Contexto</h2>
      <div class="row">
        <label>mission_id
          <input id="missionId" value="` + mission + `" placeholder="mission_..." spellcheck="false"/>
        </label>
        <button class="primary" id="btnRefresh" type="button">Atualizar</button>
        <button id="btnConnect" type="button">Conectar timeline</button>
      </div>
      <p class="muted">Leituras via Control API. Mutações só por comandos/eventos tipados. Tokens e segredos nunca aparecem aqui.</p>
      <div class="errbox" id="globalError"></div>
    </section>
    <section>
      <h2>Overview</h2>
      <div id="overview" class="muted">carregue uma missão</div>
    </section>
    <section>
      <h2>Perguntas pendentes</h2>
      <div id="questions" class="list muted">nenhuma carregada</div>
    </section>
  </div>
  <div>
    <section>
      <h2>Timeline (SSE)</h2>
      <div class="row">
        <label>after_sequence
          <input id="afterSeq" value="0" spellcheck="false"/>
        </label>
        <label>filtro kind
          <input id="eventKind" placeholder="opcional" spellcheck="false"/>
        </label>
      </div>
      <div id="timeline" class="timeline">aguardando conexão…</div>
    </section>
    <section>
      <h2>Resposta correlacionada</h2>
      <div class="row">
        <label>question_id
          <input id="answerQuestionId" spellcheck="false"/>
        </label>
        <label>expected_revision
          <input id="answerRevision" type="number" min="1" value="1"/>
        </label>
        <label>kind
          <select id="answerKind">
            <option value="TEXT">TEXT</option>
            <option value="OPTION">OPTION</option>
            <option value="MULTI_OPTION">MULTI_OPTION</option>
            <option value="CONFIRM">CONFIRM</option>
            <option value="SKIP">SKIP</option>
          </select>
        </label>
      </div>
      <label>texto / option_ids (CSV)
        <textarea id="answerBody" placeholder="texto livre ou option_a,option_b"></textarea>
      </label>
      <div class="row" style="margin-top:8px">
        <label>idempotency_key
          <input id="answerIdem" spellcheck="false"/>
        </label>
        <button class="primary" id="btnAnswer" type="button">Enviar resposta</button>
      </div>
      <div class="okbox" id="answerOk"></div>
      <div class="errbox" id="answerErr"></div>
    </section>
  </div>
</main>
<script>
(function () {
  const API_BASE = ` + jsonString(base) + `;
  const inspectBase = API_BASE + "/inspect";
  const controlBase = API_BASE + "/control";

  const el = (id) => document.getElementById(id);
  let es = null;
  let lastSeq = 0;

  function setError(msg) {
    el("globalError").textContent = msg || "";
  }
  function fmtTime(v) {
    if (!v) return "—";
    try { return new Date(v).toISOString(); } catch { return String(v); }
  }
  async function getJSON(url) {
    const res = await fetch(url, { headers: { "Accept": "application/json" }, cache: "no-store" });
    const text = await res.text();
    let body = null;
    try { body = text ? JSON.parse(text) : null; } catch { body = { raw: text }; }
    if (!res.ok) {
      const code = body && body.error && body.error.code ? body.error.code : ("http_" + res.status);
      const message = body && body.error && body.error.message ? body.error.message : res.statusText;
      throw new Error(code + ": " + message);
    }
    return body;
  }
  async function postJSON(url, payload) {
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", "Accept": "application/json" },
      body: JSON.stringify(payload),
      cache: "no-store"
    });
    const text = await res.text();
    let body = null;
    try { body = text ? JSON.parse(text) : null; } catch { body = { raw: text }; }
    if (!res.ok) {
      const code = body && body.error && body.error.code ? body.error.code : ("http_" + res.status);
      const message = body && body.error && body.error.message ? body.error.message : res.statusText;
      throw new Error(code + ": " + message);
    }
    return body;
  }

  function renderOverview(o) {
    const m = o.mission;
    let html = '<dl class="kv">';
    html += '<dt>runtime</dt><dd>' + esc((o.runtime && o.runtime.name) || "") + " " + esc((o.runtime && o.runtime.version) || "") + "</dd>";
    html += '<dt>process_mode</dt><dd class="status-' + esc(o.process_mode || "") + '">' + esc(o.process_mode || "—") + "</dd>";
    html += '<dt>control_revision</dt><dd>' + esc(String(o.control_revision ?? "—")) + "</dd>";
    html += '<dt>event_head</dt><dd>' + esc(String(o.event_head_sequence ?? "—")) + "</dd>";
    html += '<dt>pending_commands</dt><dd>' + esc(String(o.pending_commands ?? 0)) + "</dd>";
    html += '<dt>pending_questions</dt><dd>' + esc(String(o.pending_operator_questions ?? 0)) + "</dd>";
    html += '<dt>generated_at</dt><dd>' + esc(fmtTime(o.generated_at)) + "</dd>";
    if (m) {
      html += '<dt>mission</dt><dd>' + esc(m.mission_id) + "</dd>";
      html += '<dt>status</dt><dd class="status-' + esc(m.status || "") + '">' + esc(m.status || "—") + "</dd>";
      html += '<dt>revision</dt><dd>' + esc(String(m.active_revision ?? "—")) + " (" + esc(m.active_revision_id || "") + ")</dd>";
      html += '<dt>purpose</dt><dd>' + esc(m.purpose || "") + "</dd>";
      html += '<dt>dispatch</dt><dd>' + esc(m.dispatch_mode || "") + " allows_new=" + esc(String(!!m.dispatch_allows_new)) + "</dd>";
      if (m.agenda) {
        html += '<dt>agenda</dt><dd>total=' + esc(String(m.agenda.total||0))
          + " ready=" + esc(String(m.agenda.ready||0))
          + " running=" + esc(String(m.agenda.running||0))
          + " waiting=" + esc(String(m.agenda.waiting||0))
          + " terminal=" + esc(String(m.agenda.terminal||0)) + "</dd>";
      }
    } else {
      html += '<dt>mission</dt><dd class="muted">não selecionada / não encontrada</dd>';
    }
    html += "</dl>";
    if (m && Array.isArray(m.operations) && m.operations.length) {
      html += '<div class="list" style="margin-top:10px">';
      m.operations.slice(0, 20).forEach(function (op) {
        html += '<div class="card"><div class="id">' + esc(op.operation_id) + "</div>"
          + "<div>state <strong>" + esc(op.state) + "</strong> attempt=" + esc(String(op.attempt||0)) + "</div>"
          + '<div class="muted">inquiry ' + esc(op.inquiry_id||"") + " · spec " + esc(op.spec_id||"") + "</div></div>";
      });
      html += "</div>";
    }
    el("overview").innerHTML = html;
    el("clockMeta").textContent = "head=" + (o.event_head_sequence ?? "—");
  }

  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return ({ "&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;" })[c];
    });
  }

  function renderQuestions(items) {
    if (!items || !items.length) {
      el("questions").innerHTML = '<span class="muted">nenhuma pergunta no filtro atual</span>';
      return;
    }
    let html = "";
    items.forEach(function (q) {
      html += '<div class="card">';
      html += '<h3>' + esc(q.prompt || q.question_text || q.text || "(sem prompt)") + "</h3>";
      html += '<div class="id">' + esc(q.question_id || q.id) + " · rev " + esc(String(q.revision||1)) + " · " + esc(q.status||"") + "</div>";
      if (q.reason) html += '<div class="muted">' + esc(q.reason) + "</div>";
      if (Array.isArray(q.options) && q.options.length) {
        html += '<div class="muted">options: ' + esc(q.options.map(function(o){ return o.id || o.option_id || o; }).join(", ")) + "</div>";
      }
      html += '<div class="ops"><button type="button" data-fill="' + esc(q.question_id || q.id) + '" data-rev="' + esc(String(q.revision||1)) + '">Responder</button></div>';
      html += "</div>";
    });
    el("questions").innerHTML = html;
    el("questions").querySelectorAll("button[data-fill]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        el("answerQuestionId").value = btn.getAttribute("data-fill") || "";
        el("answerRevision").value = btn.getAttribute("data-rev") || "1";
        el("answerIdem").value = "dash_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2, 8);
        el("answerBody").focus();
      });
    });
  }

  async function refresh() {
    setError("");
    const missionId = el("missionId").value.trim();
    try {
      const health = await getJSON(inspectBase + "/health");
      el("headerMeta").textContent = "store=" + (health.status || health.store || "ok");
      const overviewURL = inspectBase + "/overview" + (missionId ? ("?mission_id=" + encodeURIComponent(missionId)) : "");
      const overview = await getJSON(overviewURL);
      renderOverview(overview);
      if (missionId) {
        const q = await getJSON(controlBase + "/questions?mission_id=" + encodeURIComponent(missionId) + "&status=PENDING");
        renderQuestions(q.questions || []);
      } else {
        el("questions").innerHTML = '<span class="muted">informe mission_id</span>';
      }
    } catch (err) {
      setError(String(err.message || err));
    }
  }

  function appendTimeline(line) {
    const box = el("timeline");
    if (box.dataset.empty === "1" || box.textContent.indexOf("aguardando") === 0) {
      box.textContent = "";
      box.dataset.empty = "0";
    }
    box.textContent += line + "\n";
    box.scrollTop = box.scrollHeight;
  }

  function connectStream() {
    if (es) { es.close(); es = null; }
    const after = el("afterSeq").value.trim() || "0";
    const kind = el("eventKind").value.trim();
    let url = inspectBase + "/events/stream?after_sequence=" + encodeURIComponent(after) + "&poll_ms=400&limit=50";
    if (kind) url += "&kind=" + encodeURIComponent(kind);
    el("timeline").textContent = "conectando " + url + "…\n";
    el("timeline").dataset.empty = "0";
    es = new EventSource(url);
    el("streamBadge").textContent = "SSE connecting";
    el("streamBadge").className = "badge";
    es.addEventListener("ready", function (ev) {
      el("streamBadge").textContent = "SSE live";
      el("streamBadge").className = "badge live";
      appendTimeline("# ready " + ev.data);
    });
    es.addEventListener("event", function (ev) {
      try {
        const data = JSON.parse(ev.data);
        if (data.sequence) {
          lastSeq = data.sequence;
          el("afterSeq").value = String(lastSeq);
        }
        appendTimeline(String(data.sequence||"?") + " " + (data.kind||"?") + " " + (data.id||"") + " " + (data.payload_ref||""));
      } catch {
        appendTimeline(ev.data);
      }
    });
    es.addEventListener("page", function (ev) {
      appendTimeline("# page " + ev.data);
    });
    es.addEventListener("error", function (ev) {
      if (ev && ev.data) appendTimeline("# error " + ev.data);
    });
    es.onerror = function () {
      el("streamBadge").textContent = "SSE error/retry";
      el("streamBadge").className = "badge err";
    };
  }

  async function submitAnswer() {
    el("answerOk").textContent = "";
    el("answerErr").textContent = "";
    const qid = el("answerQuestionId").value.trim();
    const rev = Number(el("answerRevision").value || "0");
    const kind = el("answerKind").value;
    const idem = el("answerIdem").value.trim() || ("dash_" + Date.now().toString(36));
    const bodyRaw = el("answerBody").value.trim();
    if (!qid || !rev) {
      el("answerErr").textContent = "question_id e expected_revision são obrigatórios";
      return;
    }
    const payload = {
      schema_version: 1,
      idempotency_key: idem,
      expected_question_revision: rev,
      kind: kind
    };
    if (kind === "TEXT" || kind === "CONFIRM" || kind === "SKIP") {
      payload.text = bodyRaw;
    } else {
      payload.option_ids = bodyRaw.split(",").map(function (s) { return s.trim(); }).filter(Boolean);
    }
    try {
      const res = await postJSON(controlBase + "/questions/" + encodeURIComponent(qid) + "/answers", payload);
      el("answerOk").textContent = "accepted event=" + (res.event_id||"") + " answer=" + (res.answer_id||"") + " disposition=" + JSON.stringify(res.disposition||{});
      await refresh();
    } catch (err) {
      el("answerErr").textContent = String(err.message || err);
    }
  }

  el("btnRefresh").addEventListener("click", refresh);
  el("btnConnect").addEventListener("click", connectStream);
  el("btnAnswer").addEventListener("click", submitAnswer);
  if (el("missionId").value.trim()) {
    refresh();
  }
})();
</script>
</body>
</html>`
}

func jsonString(v string) string {
	// minimal JSON string for embedding into JS string literal with backticks alternative —
	// we use a double-quoted JS string.
	b := strings.Builder{}
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				const hex = "0123456789abcdef"
				b.WriteByte(hex[r>>4])
				b.WriteByte(hex[r&0xf])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
