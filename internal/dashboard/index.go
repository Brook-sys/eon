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
button.warn { background: #4a3a12; border-color: var(--warn); }
button.danger { background: #4a2020; border-color: var(--err); }
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
.timeline, .prebox {
  font-family: var(--mono); font-size: 11px; max-height: 420px;
  overflow: auto; white-space: pre-wrap; background: #0d1218;
  border: 1px solid var(--border); border-radius: 8px; padding: 8px;
}
.prebox { max-height: 220px; }
.errbox { color: var(--err); font-size: 12px; margin-top: 8px; white-space: pre-wrap; }
.okbox { color: var(--ok); font-size: 12px; margin-top: 8px; white-space: pre-wrap; }
.muted { color: var(--muted); }
.status-RUNNING, .status-ACTIVE, .status-VALIDATED, .status-APPLIED { color: var(--ok); }
.status-PAUSED, .status-OPEN { color: var(--warn); }
.status-STOPPED, .status-FAILED, .status-REJECTED { color: var(--err); }
.full { grid-column: 1 / -1; }
.hint { font-size: 12px; color: var(--muted); margin: 0 0 8px; }
.tabs { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 10px; }
.tabs button.active { border-color: var(--accent); background: #1e4d73; }
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
      <p class="muted">Leituras via Control API. Mutações só por comandos/eventos tipados e drafts versionados. Tokens e segredos nunca aparecem aqui.</p>
      <div class="errbox" id="globalError"></div>
    </section>
    <section>
      <h2>Overview</h2>
      <div id="overview" class="muted">carregue uma missão</div>
      <div class="ops" id="missionOps" style="margin-top:10px">
        <button type="button" id="btnPause" class="warn">Pause dispatch</button>
        <button type="button" id="btnResume" class="primary">Resume dispatch</button>
        <button type="button" id="btnCancel" class="danger">Cancel mission</button>
      </div>
      <div class="okbox" id="cmdOk"></div>
      <div class="errbox" id="cmdErr"></div>
    </section>
    <section>
      <h2>Perguntas pendentes</h2>
      <div id="questions" class="list muted">nenhuma carregada</div>
    </section>
    <section>
      <h2>Conhecimento</h2>
      <p class="hint">Browse somente-leitura de sources, claims, evidence e artifacts. Conteúdo livre chega redigido pela Control API; snapshot bytes não são exportados. Mutações canônicas não passam por aqui.</p>
      <div id="knowCatalog" class="muted">carregue o catálogo</div>
      <div class="row" style="margin-top:10px">
        <label>coleção
          <select id="knowKind">
            <option value="claims">claims</option>
            <option value="sources">sources</option>
            <option value="observations">observations</option>
            <option value="artifacts">artifacts</option>
          </select>
        </label>
        <label>id (detalhe)
          <input id="knowId" placeholder="claim_... / source_... / observation_... / artifact_..." spellcheck="false" style="min-width:240px"/>
        </label>
        <button class="primary" type="button" id="btnKnowList">Listar</button>
        <button type="button" id="btnKnowDetail">Detalhe</button>
        <button type="button" id="btnKnowRefresh">Catálogo</button>
      </div>
      <div class="row">
        <label><input type="checkbox" id="knowWithoutEvidence"/> só claims sem evidência</label>
        <label><input type="checkbox" id="knowStaleOnly"/> só artifacts stale</label>
      </div>
      <div class="okbox" id="knowOk"></div>
      <div class="errbox" id="knowErr"></div>
      <div id="knowList" class="list muted">nenhuma lista carregada</div>
      <div id="knowDetail" class="prebox muted" hidden></div>
    </section>
    <section>
      <h2>Inspetor de execução</h2>
      <p class="hint">Correlação somente-leitura de operation/commit/command. Conteúdo bruto de modelo chega redigido e limitado pela Control API; hashes e IDs oficiais permanecem.</p>
      <div class="row">
        <label>tipo
          <select id="inspKind">
            <option value="operation">operation</option>
            <option value="commit">commit</option>
            <option value="command">command</option>
          </select>
        </label>
        <label>id
          <input id="inspId" placeholder="operation_... / commit_... / cmd_..." spellcheck="false" style="min-width:240px"/>
        </label>
        <button class="primary" type="button" id="btnInspLoad">Inspecionar</button>
      </div>
      <div class="tabs" id="inspTabs" hidden>
        <button type="button" class="active" data-panel="summary">Resumo</button>
        <button type="button" data-panel="lineage">Linhagem</button>
        <button type="button" data-panel="changeset">ChangeSet</button>
        <button type="button" data-panel="raw">Raw / validação</button>
        <button type="button" data-panel="events">Eventos</button>
        <button type="button" data-panel="json">JSON</button>
      </div>
      <div id="inspSummary" class="muted">informe um id e carregue</div>
      <div id="inspLineage" class="prebox muted" hidden></div>
      <div id="inspChangeset" class="prebox muted" hidden></div>
      <div id="inspRaw" class="prebox muted" hidden></div>
      <div id="inspEvents" class="prebox muted" hidden></div>
      <div id="inspJSON" class="prebox muted" hidden></div>
      <div class="okbox" id="inspOk"></div>
      <div class="errbox" id="inspErr"></div>
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
  <section class="full">
    <h2>Configuração versionada</h2>
    <p class="hint">Draft → validate (preview/diff) → apply com recibo. Rollback semântico re-aplica payload ancestral como nova revisão (ponteiro só avança). Segredos só por referência.</p>
    <div class="row">
      <label>scope
        <select id="cfgScope">
          <option value="INTERRUPTION">INTERRUPTION</option>
          <option value="HORIZON">HORIZON</option>
          <option value="RUNTIME">RUNTIME</option>
          <option value="SCHEDULER">SCHEDULER</option>
          <option value="CHANNELS">CHANNELS</option>
        </select>
      </label>
      <label>draft status filter
        <select id="cfgStatus">
          <option value="">(todos)</option>
          <option value="OPEN">OPEN</option>
          <option value="VALIDATED">VALIDATED</option>
          <option value="APPLIED">APPLIED</option>
          <option value="REJECTED">REJECTED</option>
        </select>
      </label>
      <button type="button" id="btnCfgRefresh">Atualizar config</button>
    </div>
    <div class="grid2">
      <div>
        <h3 class="muted" style="margin:0 0 8px;font-size:12px;text-transform:uppercase;letter-spacing:0.06em">Revisão ativa</h3>
        <div id="cfgActive" class="prebox muted">não carregada</div>
        <h3 class="muted" style="margin:12px 0 8px;font-size:12px;text-transform:uppercase;letter-spacing:0.06em">Histórico de revisões</h3>
        <div id="cfgRevisions" class="list muted">nenhum</div>
        <h3 class="muted" style="margin:12px 0 8px;font-size:12px;text-transform:uppercase;letter-spacing:0.06em">Drafts</h3>
        <div id="cfgDrafts" class="list muted">nenhum</div>
      </div>
      <div>
        <h3 class="muted" style="margin:0 0 8px;font-size:12px;text-transform:uppercase;letter-spacing:0.06em">Novo draft</h3>
        <div class="row">
          <label>based_on_revision
            <input id="cfgBasedOn" type="number" min="0" value="0"/>
          </label>
          <label>applicability
            <select id="cfgApplicability">
              <option value="">(default do escopo)</option>
              <option value="HOT">HOT</option>
              <option value="NEXT_CYCLE">NEXT_CYCLE</option>
              <option value="RESTART_REQUIRED">RESTART_REQUIRED</option>
            </select>
          </label>
        </div>
        <label>reason
          <input id="cfgReason" style="min-width:100%" placeholder="motivo do operador" spellcheck="true"/>
        </label>
        <label style="margin-top:8px">payload JSON do escopo (sem envelope)
          <textarea id="cfgPayload" style="min-height:140px" placeholder='ex. INTERRUPTION: {"version":"interruption.v1","min_priority":20,...}'></textarea>
        </label>
        <div class="ops">
          <button class="primary" type="button" id="btnCfgCreate">Criar draft</button>
          <button type="button" id="btnCfgFillDefault">Preencher default</button>
        </div>
        <div class="okbox" id="cfgOk"></div>
        <div class="errbox" id="cfgErr"></div>
        <h3 class="muted" style="margin:12px 0 8px;font-size:12px;text-transform:uppercase;letter-spacing:0.06em">Detalhe / preview</h3>
        <div id="cfgDetail" class="prebox muted">selecione um draft</div>
      </div>
    </div>
  </section>
</main>
<script>
(function () {
  const API_BASE = ` + jsonString(base) + `;
  const inspectBase = API_BASE + "/inspect";
  const controlBase = API_BASE + "/control";

  const el = (id) => document.getElementById(id);
  let es = null;
  let lastSeq = 0;
  let lastMissionRevision = null;

  function setError(msg) {
    el("globalError").textContent = msg || "";
  }
  function fmtTime(v) {
    if (!v) return "—";
    try { return new Date(v).toISOString(); } catch { return String(v); }
  }
  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return ({ "&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;" })[c];
    });
  }
  function idem() {
    return "dash_" + Date.now().toString(36) + "_" + Math.random().toString(36).slice(2, 8);
  }
  function pretty(v) {
    try { return JSON.stringify(v, null, 2); } catch { return String(v); }
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
    lastMissionRevision = m && m.active_revision != null ? Number(m.active_revision) : null;
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
      if (m.horizon) {
        const h = m.horizon;
        const needs = (Number(h.ready_count||0) <= Number(h.low_watermark||0));
        html += '<dt>horizon</dt><dd>ready=' + esc(String(h.ready_count||0))
          + " / target=" + esc(String(h.target_ready||0))
          + " max=" + esc(String(h.max_ready||0))
          + " low=" + esc(String(h.low_watermark||0))
          + " open_candidates=" + esc(String(h.open_candidates||0))
          + " policy=" + esc(h.policy_version||"")
          + (needs ? ' <span class="status-PAUSED">replenish</span>' : '') + "</dd>";
      }
      if (m.frontier) {
        const f = m.frontier;
        html += '<dt>frontier</dt><dd>total=' + esc(String(f.total||0))
          + " open=" + esc(String(f.open||0))
          + " admitted=" + esc(String(f.admitted||0))
          + " deferred=" + esc(String(f.deferred||0))
          + " abandoned=" + esc(String(f.abandoned||0))
          + " superseded=" + esc(String(f.superseded||0)) + "</dd>";
        if (Array.isArray(f.by_family) && f.by_family.length) {
          html += '<dt>frontier_families</dt><dd>' + f.by_family.map(function (row) {
            return esc(row.family || "?") + " open=" + esc(String(row.open||0)) + "/" + esc(String(row.total||0));
          }).join("; ") + "</dd>";
        }
      }
      if (m.latest_continuity_diagnosis) {
        const d = m.latest_continuity_diagnosis;
        html += '<dt>continuity_blocked</dt><dd class="status-PAUSED">' + esc(d.safe_detail || d.id || "diagnosis")
          + " · ready=" + esc(String(d.ready_count||0))
          + " open=" + esc(String(d.open_candidate_count||0))
          + " · " + esc(fmtTime(d.occurred_at)) + "</dd>";
      }
      if (m.continuity_findings) {
        const cf = m.continuity_findings;
        html += '<dt>continuity_findings</dt><dd>reports=' + esc(String(cf.total_reports||0))
          + " active=" + esc(String(cf.active_reports||0))
          + " stale=" + esc(String(cf.stale_reports||0)) + "</dd>";
        if (cf.latest) {
          const L = cf.latest;
          const fl = Array.isArray(L.findings) ? L.findings.slice(0, 6) : [];
          html += '<dt>latest_audit</dt><dd class="' + (L.stale ? 'muted' : '') + '">' + esc(L.family || L.kind || "?")
            + (L.stale ? ' · stale' : '')
            + " · artifact=" + esc(L.artifact_id || "")
            + " · " + esc(fmtTime(L.verified_at))
            + (fl.length ? (" · " + fl.map(function (line) { return esc(line); }).join("; ")) : "")
            + "</dd>";
        }
        if (Array.isArray(cf.latest_by_family) && cf.latest_by_family.length) {
          html += '<dt>audits_by_family</dt><dd>' + cf.latest_by_family.map(function (row) {
            const f0 = (Array.isArray(row.findings) && row.findings.length) ? row.findings[0] : "";
            return esc(row.family || row.kind || "?") + (row.stale ? "(stale)" : "")
              + (f0 ? (" → " + esc(f0)) : "");
          }).join("; ") + "</dd>";
        }
      }
    } else {
      html += '<dt>mission</dt><dd class="muted">não selecionada / não encontrada</dd>';
    }
    html += "</dl>";
    if (m && Array.isArray(m.operations) && m.operations.length) {
      html += '<div class="list" style="margin-top:10px">';
      m.operations.slice(0, 20).forEach(function (op) {
        const opId = op.operation_id || op.id || "";
        html += '<div class="card"><div class="id">' + esc(opId) + "</div>"
          + "<div>state <strong>" + esc(op.state) + "</strong> attempt=" + esc(String(op.attempt||0)) + "</div>"
          + '<div class="muted">inquiry ' + esc(op.inquiry_id||"") + " · spec " + esc(op.spec_id||"") + "</div>"
          + '<div class="ops"><button type="button" data-inspect-op="' + esc(opId) + '">Inspecionar</button></div></div>';
      });
      html += "</div>";
    }
    el("overview").innerHTML = html;
    el("overview").querySelectorAll("button[data-inspect-op]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        el("inspKind").value = "operation";
        el("inspId").value = btn.getAttribute("data-inspect-op") || "";
        loadInspector();
      });
    });
    el("clockMeta").textContent = "head=" + (o.event_head_sequence ?? "—");
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
        el("answerIdem").value = idem();
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
      await refreshConfig(false);
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
    const idk = el("answerIdem").value.trim() || idem();
    const bodyRaw = el("answerBody").value.trim();
    if (!qid || !rev) {
      el("answerErr").textContent = "question_id e expected_revision são obrigatórios";
      return;
    }
    const payload = {
      schema_version: 1,
      idempotency_key: idk,
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

  async function submitMissionCommand(kind) {
    el("cmdOk").textContent = "";
    el("cmdErr").textContent = "";
    const missionId = el("missionId").value.trim();
    if (!missionId) {
      el("cmdErr").textContent = "mission_id é obrigatório";
      return;
    }
    if (lastMissionRevision == null || !(lastMissionRevision > 0)) {
      el("cmdErr").textContent = "carregue o overview para obter active_revision";
      return;
    }
    const reason = window.prompt("Motivo do comando " + kind + ":", kind.toLowerCase() + " via dashboard");
    if (reason == null || !String(reason).trim()) {
      el("cmdErr").textContent = "motivo cancelado";
      return;
    }
    if (kind === "CANCEL_MISSION") {
      const ok = window.confirm("Confirma CANCEL_MISSION para " + missionId + "?");
      if (!ok) return;
    }
    try {
      const res = await postJSON(controlBase + "/commands", {
        schema_version: 1,
        idempotency_key: idem(),
        kind: kind,
        target: { mission_id: missionId },
        expected_revision: lastMissionRevision,
        reason: String(reason).trim()
      });
      el("cmdOk").textContent = "command accepted id=" + (res.command_id||"") + " receipt=" + JSON.stringify(res.receipt||{});
      await refresh();
    } catch (err) {
      el("cmdErr").textContent = String(err.message || err);
    }
  }

  function defaultPayload(scope) {
    switch (scope) {
      case "INTERRUPTION":
        return {
          version: "interruption.v1",
          min_priority: 20,
          max_pending: 3,
          max_delivered_per_window: 2,
          max_admitted_per_window: 4,
          window: 3600000000000,
          cooldown: 21600000000000,
          topic_cooldown: 86400000000000,
          quiet_start_hour: 23,
          quiet_end_hour: 7,
          urgent_priority: 90,
          min_alternatives_tried: 1,
          suppress_safe_reversible_default: true
        };
      case "HORIZON":
        return {
          schema_version: 1,
          version: "horizon.v1",
          target_ready: 4,
          low_watermark: 2,
          max_ready: 8,
          max_candidates: 64,
          max_children: 4,
          max_depth: 3,
          strategy_cooldown: 300000000000
        };
      case "RUNTIME":
        return {
          version: "runtime.v1",
          log_level: "info",
          metrics_enabled: false,
          trace_sample_per_mille: 0
        };
      case "SCHEDULER":
        return {
          version: "scheduler.v1",
          min_idle_sleep: 50000000,
          max_idle_sleep: 1000000000,
          max_cycle_duration: 0,
          max_dispatches_per_cycle: 4
        };
      case "CHANNELS":
        return {
          version: "channels.v1",
          routes: [{
            channel: "dashboard",
            destination_ref: "operator_local",
            enabled: true,
            priority: 10,
            credential_ref: { kind: "env", name: "NONE" },
            max_deliveries_per_hour: 60
          }]
        };
      default:
        return {};
    }
  }

  function fillDefaultPayload() {
    const scope = el("cfgScope").value;
    el("cfgPayload").value = pretty(defaultPayload(scope));
    el("cfgReason").value = "operator draft for " + scope;
  }

  function renderDrafts(drafts) {
    if (!drafts || !drafts.length) {
      el("cfgDrafts").innerHTML = '<span class="muted">nenhum draft neste filtro</span>';
      return;
    }
    let html = "";
    drafts.forEach(function (d) {
      html += '<div class="card">';
      html += '<div class="id">' + esc(d.draft_id) + '</div>';
      html += '<div><strong class="status-' + esc(d.status||"") + '">' + esc(d.status||"") + '</strong> · ' + esc(d.scope||"") + ' · app=' + esc(d.applicability||"") + '</div>';
      html += '<div class="muted">based_on=' + esc(String(d.based_on_revision??0)) + ' · ' + esc(d.reason||"") + '</div>';
      html += '<div class="ops">';
      html += '<button type="button" data-draft="' + esc(d.draft_id) + '" data-act="detail">Detalhe</button>';
      if (d.status === "OPEN") {
        html += '<button type="button" data-draft="' + esc(d.draft_id) + '" data-act="validate">Validate</button>';
      }
      if (d.status === "VALIDATED") {
        html += '<button class="primary" type="button" data-draft="' + esc(d.draft_id) + '" data-act="apply">Apply</button>';
      }
      if (d.status === "APPLIED") {
        html += '<button type="button" data-draft="' + esc(d.draft_id) + '" data-act="receipt">Receipt</button>';
      }
      html += '</div></div>';
    });
    el("cfgDrafts").innerHTML = html;
    el("cfgDrafts").querySelectorAll("button[data-draft]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        const id = btn.getAttribute("data-draft");
        const act = btn.getAttribute("data-act");
        if (act === "detail") loadDraftDetail(id);
        else if (act === "validate") validateDraft(id);
        else if (act === "apply") applyDraft(id);
        else if (act === "receipt") loadReceipt(id);
      });
    });
  }

  function renderRevisions(revisions, activeID) {
    const list = Array.isArray(revisions) ? revisions.slice() : [];
    list.sort(function (a, b) { return (b.revision || 0) - (a.revision || 0); });
    if (!list.length) {
      el("cfgRevisions").innerHTML = '<span class="muted">nenhuma revisão</span>';
      return;
    }
    let html = "";
    list.forEach(function (rev) {
      const id = rev.revision_id || "";
      const isActive = activeID && id === activeID;
      html += '<div class="item">';
      html += '<div><strong>#' + esc(String(rev.revision || "?")) + '</strong> ' + esc(id);
      if (isActive) html += ' <span class="muted">(ativa)</span>';
      html += '</div>';
      html += '<div class="muted">' + esc(rev.reason || "") + '</div>';
      html += '<div class="ops">';
      html += '<button type="button" data-revid="' + esc(id) + '" data-act="detail">Detalhe</button>';
      if (!isActive) {
        html += '<button type="button" data-revid="' + esc(id) + '" data-act="rollback">Rollback semântico</button>';
      }
      html += '</div></div>';
    });
    el("cfgRevisions").innerHTML = html;
    el("cfgRevisions").querySelectorAll("button[data-revid]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        const id = btn.getAttribute("data-revid");
        const act = btn.getAttribute("data-act");
        if (act === "detail") loadRevisionDetail(id);
        else if (act === "rollback") rollbackRevision(id);
      });
    });
  }

  async function refreshConfig(showErrors) {
    el("cfgOk").textContent = "";
    if (showErrors) el("cfgErr").textContent = "";
    const scope = el("cfgScope").value;
    const status = el("cfgStatus").value;
    let activeID = "";
    try {
      try {
        const active = await getJSON(controlBase + "/config/revisions/active?scope=" + encodeURIComponent(scope));
        el("cfgActive").textContent = pretty(active.revision || active);
        el("cfgActive").className = "prebox";
        if (active.revision && active.revision.revision != null) {
          el("cfgBasedOn").value = String(active.revision.revision);
        }
        if (active.revision && active.revision.revision_id) activeID = active.revision.revision_id;
      } catch (err) {
        el("cfgActive").textContent = "sem revisão ativa: " + String(err.message || err);
        el("cfgActive").className = "prebox muted";
      }
      try {
        const revs = await getJSON(controlBase + "/config/revisions?scope=" + encodeURIComponent(scope));
        renderRevisions(revs.revisions || [], activeID);
      } catch (err) {
        el("cfgRevisions").innerHTML = '<span class="muted">histórico indisponível: ' + esc(String(err.message || err)) + '</span>';
      }
      let listURL = controlBase + "/config/drafts?scope=" + encodeURIComponent(scope);
      if (status) listURL += "&status=" + encodeURIComponent(status);
      const listed = await getJSON(listURL);
      renderDrafts(listed.drafts || []);
    } catch (err) {
      if (showErrors !== false) el("cfgErr").textContent = String(err.message || err);
      else el("cfgDrafts").innerHTML = '<span class="muted">config indisponível: ' + esc(String(err.message || err)) + '</span>';
    }
  }

  function loadRevisionDetail(id) {
    const scope = el("cfgScope").value;
    getJSON(controlBase + "/config/revisions?scope=" + encodeURIComponent(scope)).then(function (body) {
      const list = body.revisions || [];
      const found = list.find(function (r) { return r.revision_id === id; });
      el("cfgDetail").textContent = pretty(found || { error: "revision not in list", id: id });
      el("cfgDetail").className = "prebox";
    }).catch(function (err) {
      el("cfgErr").textContent = String(err.message || err);
    });
  }

  async function rollbackRevision(id) {
    el("cfgOk").textContent = "";
    el("cfgErr").textContent = "";
    const scope = el("cfgScope").value;
    if (!window.confirm("Rollback semântico: re-aplicar payload de " + id + " como NOVA revisão (histórico preservado)?")) return;
    try {
      const body = await postJSON(controlBase + "/config/revisions/rollback", {
        schema_version: 1,
        scope: scope,
        target_revision_id: id,
        reason: "dashboard semantic rollback"
      });
      el("cfgDetail").textContent = pretty(body);
      el("cfgDetail").className = "prebox";
      el("cfgOk").textContent = "rollback → revision=" + ((body.revision && body.revision.revision_id) || "") + " receipt=" + ((body.receipt && body.receipt.state) || "");
      await refreshConfig(true);
    } catch (err) {
      el("cfgErr").textContent = String(err.message || err);
    }
  }

  async function loadDraftDetail(id) {
    el("cfgErr").textContent = "";
    try {
      const body = await getJSON(controlBase + "/config/drafts/" + encodeURIComponent(id));
      el("cfgDetail").textContent = pretty(body);
      el("cfgDetail").className = "prebox";
    } catch (err) {
      el("cfgErr").textContent = String(err.message || err);
    }
  }

  async function validateDraft(id) {
    el("cfgOk").textContent = "";
    el("cfgErr").textContent = "";
    try {
      const body = await postJSON(controlBase + "/config/drafts/" + encodeURIComponent(id) + "/validate", {});
      el("cfgDetail").textContent = pretty(body);
      el("cfgDetail").className = "prebox";
      el("cfgOk").textContent = "validated draft=" + id + " blocked=" + String(!!(body.preview && body.preview.blocked));
      await refreshConfig(true);
    } catch (err) {
      el("cfgErr").textContent = String(err.message || err);
    }
  }

  async function applyDraft(id) {
    el("cfgOk").textContent = "";
    el("cfgErr").textContent = "";
    if (!window.confirm("Aplicar draft " + id + " como revisão imutável?")) return;
    try {
      const body = await postJSON(controlBase + "/config/drafts/" + encodeURIComponent(id) + "/apply", {});
      el("cfgDetail").textContent = pretty(body);
      el("cfgDetail").className = "prebox";
      el("cfgOk").textContent = "applied revision=" + ((body.revision && body.revision.revision_id) || "") + " receipt=" + ((body.receipt && body.receipt.state) || "");
      await refreshConfig(true);
    } catch (err) {
      el("cfgErr").textContent = String(err.message || err);
    }
  }

  async function loadReceipt(id) {
    el("cfgErr").textContent = "";
    try {
      const body = await getJSON(controlBase + "/config/drafts/" + encodeURIComponent(id) + "/receipt");
      el("cfgDetail").textContent = pretty(body);
      el("cfgDetail").className = "prebox";
    } catch (err) {
      el("cfgErr").textContent = String(err.message || err);
    }
  }

  async function createDraft() {
    el("cfgOk").textContent = "";
    el("cfgErr").textContent = "";
    const scope = el("cfgScope").value;
    const reason = el("cfgReason").value.trim();
    const basedOn = Number(el("cfgBasedOn").value || "0");
    const applicability = el("cfgApplicability").value;
    let payload;
    try {
      payload = JSON.parse(el("cfgPayload").value || "{}");
    } catch (err) {
      el("cfgErr").textContent = "payload JSON inválido: " + String(err.message || err);
      return;
    }
    if (!reason) {
      el("cfgErr").textContent = "reason é obrigatório";
      return;
    }
    const body = {
      schema_version: 1,
      scope: scope,
      based_on_revision: basedOn,
      reason: reason
    };
    if (applicability) body.applicability = applicability;
    if (scope === "INTERRUPTION") body.interruption = payload;
    else if (scope === "HORIZON") body.horizon = payload;
    else if (scope === "RUNTIME") body.runtime = payload;
    else if (scope === "SCHEDULER") body.scheduler = payload;
    else if (scope === "CHANNELS") body.channels = payload;
    else {
      el("cfgErr").textContent = "escopo desconhecido";
      return;
    }
    try {
      const res = await postJSON(controlBase + "/config/drafts", body);
      el("cfgOk").textContent = "draft criado " + ((res.draft && res.draft.draft_id) || "") + " status=" + ((res.draft && res.draft.status) || "");
      el("cfgDetail").textContent = pretty(res);
      el("cfgDetail").className = "prebox";
      await refreshConfig(true);
    } catch (err) {
      el("cfgErr").textContent = String(err.message || err);
    }
  }

  function showInspPanel(name) {
    const panels = {
      summary: "inspSummary",
      lineage: "inspLineage",
      changeset: "inspChangeset",
      raw: "inspRaw",
      events: "inspEvents",
      json: "inspJSON"
    };
    Object.keys(panels).forEach(function (key) {
      const node = el(panels[key]);
      if (!node) return;
      node.hidden = key !== name;
      if (key === "summary" && name !== "summary") {
        // keep summary block visible only for summary tab; for others hide
      }
    });
    // summary is a div, not always prebox; ensure visibility toggles work
    el("inspSummary").hidden = name !== "summary";
    el("inspTabs").querySelectorAll("button[data-panel]").forEach(function (btn) {
      btn.className = btn.getAttribute("data-panel") === name ? "active" : "";
    });
  }

  function renderInspector(kind, body) {
    el("inspTabs").hidden = false;
    el("inspJSON").textContent = pretty(body);
    el("inspJSON").className = "prebox";

    if (kind === "operation") {
      const op = body.operation || {};
      let sum = '<dl class="kv">';
      sum += '<dt>operation</dt><dd>' + esc(op.id || op.operation_id || "") + "</dd>";
      sum += '<dt>state</dt><dd class="status-' + esc(op.state || "") + '">' + esc(op.state || "—") + "</dd>";
      sum += '<dt>attempt</dt><dd>' + esc(String(op.attempt || 0)) + "</dd>";
      sum += '<dt>spec</dt><dd>' + esc(op.spec_id || (body.spec && body.spec.id) || "") + "</dd>";
      sum += '<dt>inquiry</dt><dd>' + esc(op.inquiry_id || (body.inquiry && body.inquiry.id) || "") + "</dd>";
      sum += '<dt>idempotency</dt><dd>' + esc(op.idempotency_key || "") + "</dd>";
      sum += '<dt>commits</dt><dd>' + esc(String((body.commits || []).length)) + "</dd>";
      sum += '<dt>raw_outputs</dt><dd>' + esc(String((body.raw_model_outputs || []).length)) + "</dd>";
      sum += '<dt>validations</dt><dd>' + esc(String((body.validation_receipts || []).length)) + "</dd>";
      if (body.redaction) {
        sum += '<dt>redaction</dt><dd>applied=' + esc(String(!!body.redaction.applied))
          + " secrets=" + esc(String(body.redaction.secret_matches || 0))
          + " truncated_bytes=" + esc(String(body.redaction.truncated_bytes || 0)) + "</dd>";
      }
      sum += "</dl>";
      if (Array.isArray(body.commits) && body.commits.length) {
        sum += '<div class="ops" style="margin-top:8px">';
        body.commits.forEach(function (c) {
          sum += '<button type="button" data-inspect-commit="' + esc(c.id) + '">commit ' + esc(c.id) + "</button>";
        });
        sum += "</div>";
      }
      el("inspSummary").innerHTML = sum;
      el("inspSummary").className = "";

      let lineage = {
        operation: op,
        spec: body.spec || null,
        inquiry: body.inquiry || null,
        question: body.question || null,
        idempotency: body.idempotency || null,
        head_commit: body.head_commit || null
      };
      el("inspLineage").textContent = pretty(lineage);
      el("inspLineage").className = "prebox";

      el("inspChangeset").textContent = pretty({
        proposed_change_sets: body.proposed_change_sets || [],
        accepted_change_sets: body.accepted_change_sets || [],
        commits: body.commits || [],
        commit_receipts: body.commit_receipts || []
      });
      el("inspChangeset").className = "prebox";

      el("inspRaw").textContent = pretty({
        raw_model_outputs: body.raw_model_outputs || [],
        validation_receipts: body.validation_receipts || [],
        redaction: body.redaction || null
      });
      el("inspRaw").className = "prebox";

      el("inspEvents").textContent = pretty(body.events || []);
      el("inspEvents").className = "prebox";

      el("inspSummary").querySelectorAll("button[data-inspect-commit]").forEach(function (btn) {
        btn.addEventListener("click", function () {
          el("inspKind").value = "commit";
          el("inspId").value = btn.getAttribute("data-inspect-commit") || "";
          loadInspector();
        });
      });
    } else if (kind === "commit") {
      const c = body.commit || {};
      let sum = '<dl class="kv">';
      sum += '<dt>commit</dt><dd>' + esc(c.id || "") + "</dd>";
      sum += '<dt>version</dt><dd>' + esc(String(c.version || "")) + "</dd>";
      sum += '<dt>base</dt><dd>' + esc(c.base_commit_id || "") + "</dd>";
      sum += '<dt>accepted</dt><dd>' + esc(c.accepted_change_set_id || "") + "</dd>";
      sum += '<dt>mission_revision</dt><dd>' + esc(c.mission_revision_id || "") + "</dd>";
      if (body.proposed_change_set && body.proposed_change_set.operation_id) {
        sum += '<dt>operation</dt><dd>' + esc(body.proposed_change_set.operation_id) + "</dd>";
      }
      sum += "</dl>";
      if (body.proposed_change_set && body.proposed_change_set.operation_id) {
        sum += '<div class="ops"><button type="button" data-inspect-op="' + esc(body.proposed_change_set.operation_id) + '">Abrir operation</button></div>';
      }
      el("inspSummary").innerHTML = sum;
      el("inspSummary").className = "";
      el("inspLineage").textContent = pretty({
        commit: c,
        commit_receipt: body.commit_receipt || null,
        accepted_change_set: body.accepted_change_set || null
      });
      el("inspLineage").className = "prebox";
      el("inspChangeset").textContent = pretty({
        proposed_change_set: body.proposed_change_set || null,
        accepted_change_set: body.accepted_change_set || null
      });
      el("inspChangeset").className = "prebox";
      el("inspRaw").textContent = pretty({ validation_receipts: body.validation_receipts || [] });
      el("inspRaw").className = "prebox";
      el("inspEvents").textContent = pretty(body.events || []);
      el("inspEvents").className = "prebox";
      el("inspSummary").querySelectorAll("button[data-inspect-op]").forEach(function (btn) {
        btn.addEventListener("click", function () {
          el("inspKind").value = "operation";
          el("inspId").value = btn.getAttribute("data-inspect-op") || "";
          loadInspector();
        });
      });
    } else {
      // command
      const cmd = body.command || {};
      const receipt = body.receipt || {};
      let sum = '<dl class="kv">';
      sum += '<dt>command</dt><dd>' + esc(cmd.id || cmd.command_id || "") + "</dd>";
      sum += '<dt>kind</dt><dd>' + esc(cmd.kind || "") + "</dd>";
      sum += '<dt>receipt_state</dt><dd>' + esc(receipt.state || "") + "</dd>";
      sum += '<dt>result_ref</dt><dd>' + esc(receipt.result_ref || "") + "</dd>";
      sum += '<dt>failure</dt><dd>' + esc(receipt.failure_code || "—") + "</dd>";
      sum += "</dl>";
      el("inspSummary").innerHTML = sum;
      el("inspSummary").className = "";
      el("inspLineage").textContent = pretty({ command: cmd, receipt: receipt });
      el("inspLineage").className = "prebox";
      el("inspChangeset").textContent = pretty({ note: "commands do not produce knowledge changesets" });
      el("inspChangeset").className = "prebox muted";
      el("inspRaw").textContent = pretty({ note: "no model raw output on commands" });
      el("inspRaw").className = "prebox muted";
      el("inspEvents").textContent = pretty(body.events || []);
      el("inspEvents").className = "prebox";
    }
    showInspPanel("summary");
  }

  async function loadInspector() {
    el("inspOk").textContent = "";
    el("inspErr").textContent = "";
    const kind = el("inspKind").value;
    const id = el("inspId").value.trim();
    if (!id) {
      el("inspErr").textContent = "id é obrigatório";
      return;
    }
    let path = "";
    if (kind === "operation") path = "/operations/" + encodeURIComponent(id);
    else if (kind === "commit") path = "/commits/" + encodeURIComponent(id);
    else if (kind === "command") path = "/commands/" + encodeURIComponent(id);
    else {
      el("inspErr").textContent = "tipo desconhecido";
      return;
    }
    try {
      const body = await getJSON(inspectBase + path);
      renderInspector(kind, body);
      el("inspOk").textContent = "carregado " + kind + " " + id;
      // scroll inspector into view for operator workflow
      el("inspSummary").scrollIntoView({ behavior: "smooth", block: "nearest" });
    } catch (err) {
      el("inspErr").textContent = String(err.message || err);
    }
  }

  // Timeline click: if an event line contains known ids, prefill inspector.
  el("timeline").addEventListener("click", function () {
    /* selection-based optional; operators use explicit id fields */
  });

  el("btnRefresh").addEventListener("click", refresh);
  el("btnConnect").addEventListener("click", connectStream);
  el("btnAnswer").addEventListener("click", submitAnswer);
  el("btnPause").addEventListener("click", function () { submitMissionCommand("PAUSE_MISSION"); });
  el("btnResume").addEventListener("click", function () { submitMissionCommand("RESUME_MISSION"); });
  el("btnCancel").addEventListener("click", function () { submitMissionCommand("CANCEL_MISSION"); });
  el("btnCfgRefresh").addEventListener("click", function () { refreshConfig(true); });
  el("btnCfgCreate").addEventListener("click", createDraft);
  el("btnCfgFillDefault").addEventListener("click", fillDefaultPayload);
  el("cfgScope").addEventListener("change", function () { refreshConfig(true); });
  el("cfgStatus").addEventListener("change", function () { refreshConfig(true); });
  el("btnInspLoad").addEventListener("click", loadInspector);
  el("inspTabs").querySelectorAll("button[data-panel]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      showInspPanel(btn.getAttribute("data-panel") || "summary");
    });
  });

  async function refreshKnowledgeCatalog() {
    el("knowErr").textContent = "";
    try {
      const body = await getJSON(inspectBase + "/knowledge");
      let html = '<dl class="kv">';
      html += '<dt>sources</dt><dd>' + esc(String(body.sources || 0)) + "</dd>";
      html += '<dt>source_versions</dt><dd>' + esc(String(body.source_versions || 0)) + "</dd>";
      html += '<dt>observations</dt><dd>' + esc(String(body.observations || 0)) + "</dd>";
      html += '<dt>claims</dt><dd>' + esc(String(body.claims || 0)) + "</dd>";
      html += '<dt>evidence_links</dt><dd>' + esc(String(body.evidence_links || 0)) + "</dd>";
      html += '<dt>artifacts</dt><dd>' + esc(String(body.artifacts || 0)) + "</dd>";
      html += '<dt>stale_artifacts</dt><dd>' + esc(String(body.stale_artifacts || 0)) + "</dd>";
      html += '<dt>claims_without_evidence</dt><dd>' + esc(String(body.claims_without_evidence || 0)) + "</dd>";
      html += '<dt>supporting</dt><dd>' + esc(String(body.supporting_evidence_links || 0)) + "</dd>";
      html += '<dt>contradicting</dt><dd>' + esc(String(body.contradicting_evidence_links || 0)) + "</dd>";
      html += "</dl>";
      el("knowCatalog").innerHTML = html;
      el("knowCatalog").className = "";
      el("knowOk").textContent = "catálogo atualizado";
    } catch (err) {
      el("knowErr").textContent = String(err.message || err);
    }
  }

  function knowledgeCollectionPath(kind) {
    if (kind === "claims") return "/knowledge/claims";
    if (kind === "sources") return "/knowledge/sources";
    if (kind === "observations") return "/knowledge/observations";
    if (kind === "artifacts") return "/knowledge/artifacts";
    return "";
  }

  async function listKnowledge() {
    el("knowErr").textContent = "";
    el("knowOk").textContent = "";
    el("knowDetail").hidden = true;
    const kind = el("knowKind").value;
    const path = knowledgeCollectionPath(kind);
    if (!path) {
      el("knowErr").textContent = "coleção desconhecida";
      return;
    }
    const params = new URLSearchParams();
    params.set("limit", "50");
    if (kind === "claims" && el("knowWithoutEvidence").checked) params.set("without_evidence", "true");
    if (kind === "artifacts" && el("knowStaleOnly").checked) params.set("stale", "true");
    try {
      const body = await getJSON(inspectBase + path + "?" + params.toString());
      const items = body.items || [];
      if (!items.length) {
        el("knowList").textContent = "lista vazia (total=" + String(body.total || 0) + ")";
        el("knowList").className = "list muted";
        el("knowOk").textContent = "listado " + kind + " total=" + String(body.total || 0);
        return;
      }
      let html = "";
      items.forEach(function (item) {
        const id = item.id || "";
        html += '<div class="card">';
        html += '<div class="id">' + esc(id) + "</div>";
        if (kind === "claims") {
          html += "<h3>" + esc(item.proposition || "") + "</h3>";
          html += '<div class="muted">evidence=' + esc(String(item.evidence_count || 0))
            + " supports=" + esc(String(item.supports || 0))
            + " contradicts=" + esc(String(item.contradicts || 0))
            + (item.without_evidence ? " · SEM EVIDÊNCIA" : "") + "</div>";
        } else if (kind === "sources") {
          html += "<h3>" + esc(item.kind || "") + " · " + esc(item.locator || "") + "</h3>";
          html += '<div class="muted">versions=' + esc(String(item.versions || 0)) + "</div>";
        } else if (kind === "observations") {
          html += "<h3>" + esc(item.statement || "") + "</h3>";
          html += '<div class="muted">provenance=' + esc(item.provenance || "") + "</div>";
        } else {
          html += "<h3>" + esc(item.kind || "artifact") + (item.stale ? " · STALE" : "") + "</h3>";
          html += '<div class="muted">deps=' + esc(String(item.dependency_count || 0))
            + " bytes=" + esc(String(item.content_bytes || 0)) + "</div>";
        }
        html += '<div class="ops"><button type="button" data-know-id="' + esc(id) + '">Abrir detalhe</button></div>';
        html += "</div>";
      });
      el("knowList").innerHTML = html;
      el("knowList").className = "list";
      el("knowList").querySelectorAll("button[data-know-id]").forEach(function (btn) {
        btn.addEventListener("click", function () {
          el("knowId").value = btn.getAttribute("data-know-id") || "";
          loadKnowledgeDetail();
        });
      });
      el("knowOk").textContent = "listado " + kind + " items=" + String(items.length) + " total=" + String(body.total || 0);
    } catch (err) {
      el("knowErr").textContent = String(err.message || err);
    }
  }

  async function loadKnowledgeDetail() {
    el("knowErr").textContent = "";
    el("knowOk").textContent = "";
    const kind = el("knowKind").value;
    const id = el("knowId").value.trim();
    if (!id) {
      el("knowErr").textContent = "id é obrigatório";
      return;
    }
    const base = knowledgeCollectionPath(kind);
    if (!base) {
      el("knowErr").textContent = "coleção desconhecida";
      return;
    }
    try {
      const body = await getJSON(inspectBase + base + "/" + encodeURIComponent(id));
      el("knowDetail").hidden = false;
      el("knowDetail").textContent = pretty(body);
      el("knowDetail").className = "prebox";
      el("knowOk").textContent = "detalhe " + kind + " " + id;
      el("knowDetail").scrollIntoView({ behavior: "smooth", block: "nearest" });
    } catch (err) {
      el("knowErr").textContent = String(err.message || err);
    }
  }

  el("btnKnowList").addEventListener("click", listKnowledge);
  el("btnKnowDetail").addEventListener("click", loadKnowledgeDetail);
  el("btnKnowRefresh").addEventListener("click", refreshKnowledgeCatalog);

  // Clickable commit/operation ids in timeline rows via data attributes are filled by overview.
  fillDefaultPayload();
  refreshKnowledgeCatalog();
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
