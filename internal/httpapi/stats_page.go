package httpapi

import (
	"net/http"
)

const statsPageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AgenLeash Stats</title>
  <link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='14' fill='%2308111c'/%3E%3Cpath d='M18 20h28v6H24v10h18v6H24v16h-6V20z' fill='%2356d6c2'/%3E%3C/svg%3E">
  <style>
    :root {
      color-scheme: dark;
      --bg: #08111c;
      --bg-2: #0c1725;
      --panel: rgba(16, 28, 43, 0.9);
      --panel-2: rgba(18, 30, 45, 0.95);
      --panel-3: rgba(21, 35, 52, 0.72);
      --border: #22354d;
      --text: #e7eef8;
      --muted: #8ea1ba;
      --accent: #56d6c2;
      --accent-soft: rgba(86, 214, 194, 0.12);
      --warn: #ffcf70;
      --error: #ff7f8c;
      --success: #8ee09d;
      --info: #9fc5ff;
      --shadow: 0 18px 40px rgba(0, 0, 0, 0.28);
      --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      --sans: ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", sans-serif;
    }
    * { box-sizing: border-box; }
    [hidden] { display: none !important; }
    html, body { min-height: 100%; }
    body {
      margin: 0;
      color: var(--text);
      font-family: var(--sans);
      background:
        radial-gradient(circle at top left, rgba(86, 214, 194, 0.08), transparent 30rem),
        radial-gradient(circle at top right, rgba(255, 127, 140, 0.06), transparent 24rem),
        linear-gradient(180deg, var(--bg) 0%, var(--bg-2) 100%);
    }
    button, input, select {
      font: inherit;
    }
    button:focus-visible,
    input:focus-visible,
    select:focus-visible {
      outline: 2px solid rgba(86, 214, 194, 0.45);
      outline-offset: 2px;
    }
    .page {
      width: min(1560px, calc(100vw - 32px));
      margin: 0 auto;
      padding: 20px 0 28px;
    }
    .topbar {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 16px;
    }
    .title-block h1,
    .card h2,
    .card h3,
    p {
      margin: 0;
    }
    .title-block h1 {
      font-size: clamp(30px, 4vw, 40px);
      line-height: 0.98;
      letter-spacing: -0.05em;
    }
    .title-block p {
      margin-top: 8px;
      color: var(--muted);
      font-size: 15px;
      line-height: 1.5;
    }
    .topbar-actions {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: wrap;
    }
    .generated-pill,
    .pill {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      width: fit-content;
      border: 1px solid var(--border);
      border-radius: 999px;
      padding: 4px 10px;
      font-size: 12px;
      color: var(--muted);
      background: rgba(21, 35, 52, 0.84);
    }
    .pill-accent {
      color: var(--accent);
      border-color: rgba(86, 214, 194, 0.28);
      background: var(--accent-soft);
    }
    .pill-success {
      color: var(--success);
      border-color: rgba(142, 224, 157, 0.28);
      background: rgba(142, 224, 157, 0.12);
    }
    .pill-warn {
      color: var(--warn);
      border-color: rgba(255, 207, 112, 0.28);
      background: rgba(255, 207, 112, 0.12);
    }
    .pill-danger {
      color: var(--error);
      border-color: rgba(255, 127, 140, 0.28);
      background: rgba(255, 127, 140, 0.12);
    }
    .pill-info {
      color: var(--info);
      border-color: rgba(159, 197, 255, 0.28);
      background: rgba(159, 197, 255, 0.12);
    }
    .card {
      border: 1px solid var(--border);
      border-radius: 20px;
      background: var(--panel);
      box-shadow: var(--shadow);
      padding: 16px;
    }
    .controls-card {
      margin-bottom: 14px;
      background: linear-gradient(180deg, rgba(18, 30, 45, 0.96), rgba(12, 22, 34, 0.96));
    }
    .control-grid {
      display: grid;
      grid-template-columns: minmax(280px, 1.8fr) auto auto auto;
      gap: 12px;
      align-items: end;
    }
    .field {
      display: grid;
      gap: 8px;
      min-width: 0;
    }
    .field label {
      color: #c8d7ea;
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    input,
    select {
      width: 100%;
      padding: 12px 14px;
      border-radius: 14px;
      border: 1px solid var(--border);
      background: rgba(21, 35, 52, 0.84);
      color: var(--text);
    }
    select {
      appearance: none;
      background-image:
        linear-gradient(45deg, transparent 50%, #7fa4d6 50%),
        linear-gradient(135deg, #7fa4d6 50%, transparent 50%);
      background-position:
        calc(100% - 18px) calc(50% - 2px),
        calc(100% - 12px) calc(50% - 2px);
      background-size: 6px 6px, 6px 6px;
      background-repeat: no-repeat;
    }
    button {
      border: 1px solid var(--border);
      background: rgba(21, 35, 52, 0.84);
      color: var(--text);
      padding: 11px 14px;
      border-radius: 999px;
      cursor: pointer;
      transition: transform 140ms ease, border-color 140ms ease, background 140ms ease;
      white-space: nowrap;
    }
    button:hover {
      transform: translateY(-1px);
      border-color: rgba(86, 214, 194, 0.36);
    }
    button.primary {
      background: linear-gradient(180deg, rgba(86, 214, 194, 0.18), rgba(86, 214, 194, 0.08));
      border-color: rgba(86, 214, 194, 0.32);
    }
    button:disabled {
      opacity: 0.45;
      cursor: not-allowed;
      transform: none;
    }
    .status {
      min-height: 20px;
      margin-top: 12px;
      color: var(--muted);
      font-size: 13px;
      line-height: 1.5;
    }
    .status.success { color: var(--success); }
    .status.error { color: var(--error); }
    .status.loading { color: var(--warn); }
    .summary-grid {
      display: grid;
      gap: 12px;
      grid-template-columns: repeat(6, minmax(0, 1fr));
      margin-bottom: 14px;
    }
    .metric-card {
      border: 1px solid var(--border);
      border-radius: 16px;
      padding: 14px;
      background: linear-gradient(180deg, rgba(18, 30, 45, 0.94), rgba(12, 22, 34, 0.94));
    }
    .metric-card .label {
      color: var(--muted);
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .metric-card .value {
      margin-top: 10px;
      font-size: 30px;
      line-height: 1;
      letter-spacing: -0.05em;
      font-weight: 800;
    }
    .metric-card .hint {
      margin-top: 8px;
      color: var(--muted);
      font-size: 12px;
      line-height: 1.45;
      min-height: 2.8em;
    }
    .section-head {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 12px;
    }
    .section-title {
      font-size: 22px;
      line-height: 1.1;
      letter-spacing: -0.03em;
    }
    .section-copy {
      margin-top: 6px;
      color: var(--muted);
      font-size: 13px;
      line-height: 1.55;
    }
    .filters-grid {
      display: grid;
      grid-template-columns: minmax(220px, 1.5fr) 160px 140px 140px 110px;
      gap: 10px;
      margin-bottom: 14px;
    }
    .table-wrap {
      overflow: auto;
      border: 1px solid var(--border);
      border-radius: 16px;
      background: rgba(11, 20, 31, 0.55);
    }
    table {
      width: 100%;
      border-collapse: collapse;
      min-width: 980px;
    }
    th, td {
      padding: 12px 14px;
      text-align: left;
      border-bottom: 1px solid rgba(34, 53, 77, 0.72);
      vertical-align: top;
    }
    th {
      position: sticky;
      top: 0;
      z-index: 1;
      background: rgba(14, 24, 37, 0.96);
      color: var(--muted);
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    tbody tr:hover {
      background: rgba(86, 214, 194, 0.04);
    }
    .agent-main {
      min-width: 0;
    }
    .agent-title {
      display: flex;
      align-items: center;
      gap: 8px;
      margin-bottom: 6px;
      flex-wrap: wrap;
    }
    .agent-title strong {
      font-size: 15px;
      line-height: 1.25;
      word-break: break-word;
    }
    .agent-subtle,
    .preview {
      color: var(--muted);
      font-size: 12px;
      line-height: 1.5;
    }
    .preview {
      max-width: 42ch;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
      overflow: hidden;
      word-break: break-word;
    }
    .mono {
      font-family: var(--mono);
      font-size: 12px;
      color: #d8e4f5;
      word-break: break-all;
    }
    .pagination {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-top: 12px;
      flex-wrap: wrap;
    }
    .pagination-info {
      color: var(--muted);
      font-size: 13px;
    }
    .pagination-controls {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
    }
    .compact-list {
      display: grid;
      gap: 10px;
    }
    .insights-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 14px;
      margin-top: 14px;
      align-items: start;
    }
    .insight-wide {
      grid-column: span 2;
    }
    .mini-table-wrap {
      overflow: auto;
      border: 1px solid var(--border);
      border-radius: 16px;
      background: rgba(11, 20, 31, 0.55);
    }
    .mini-table {
      width: 100%;
      min-width: 0;
      border-collapse: collapse;
    }
    .mini-table th,
    .mini-table td {
      padding: 10px 12px;
      text-align: left;
      border-bottom: 1px solid rgba(34, 53, 77, 0.72);
      vertical-align: top;
    }
    .mini-table th {
      position: static;
      background: transparent;
      color: var(--muted);
      font-size: 10px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .mini-table tbody tr:last-child td {
      border-bottom: 0;
    }
    .mini-title {
      font-size: 14px;
      font-weight: 700;
      line-height: 1.35;
      word-break: break-word;
    }
    .subtle-line {
      margin-top: 4px;
      color: var(--muted);
      font-size: 12px;
      line-height: 1.45;
      word-break: break-word;
    }
    .compact-item {
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 12px;
      background: linear-gradient(180deg, rgba(18, 30, 45, 0.92), rgba(12, 22, 34, 0.92));
    }
    .compact-row {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 10px;
      margin-bottom: 8px;
    }
    .compact-title {
      min-width: 0;
    }
    .compact-title strong {
      display: block;
      font-size: 15px;
      line-height: 1.2;
      word-break: break-word;
    }
    .compact-meta {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 8px;
    }
    .meta-chip {
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 9px 10px;
      background: rgba(21, 35, 52, 0.72);
    }
    .meta-chip strong {
      display: block;
      margin-bottom: 4px;
      color: var(--muted);
      font-size: 10px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }
    .meta-chip span {
      display: block;
      font-size: 14px;
      line-height: 1.45;
      color: var(--text);
      word-break: break-word;
    }
    .claudecode-note {
      margin-bottom: 12px;
      padding: 10px 12px;
      border-radius: 14px;
      border: 1px solid rgba(255, 207, 112, 0.2);
      background: rgba(255, 207, 112, 0.06);
      color: #e8d59a;
      font-size: 12px;
      line-height: 1.55;
    }
    .empty {
      display: grid;
      place-items: center;
      min-height: 52vh;
      color: var(--muted);
    }
    .empty-card {
      max-width: 38rem;
      border: 1px solid var(--border);
      border-radius: 22px;
      padding: 22px;
      text-align: center;
      background: var(--panel);
      box-shadow: var(--shadow);
    }
    .empty-card h2 {
      font-size: 28px;
      line-height: 1;
      letter-spacing: -0.04em;
      margin: 0 0 12px;
    }
    .empty-card p,
    .empty-inline {
      color: var(--muted);
      line-height: 1.6;
      font-size: 13px;
    }
    .empty-inline {
      padding: 8px 0 2px;
    }
    @media (max-width: 1440px) {
      .summary-grid {
        grid-template-columns: repeat(3, minmax(0, 1fr));
      }
      .insight-wide {
        grid-column: span 2;
      }
    }
    @media (max-width: 900px) {
      .page {
        width: min(100vw - 20px, 100%);
      }
      .topbar,
      .control-grid,
      .filters-grid,
      .pagination {
        grid-template-columns: 1fr;
      }
      .topbar {
        flex-direction: column;
      }
      .summary-grid {
        grid-template-columns: repeat(2, minmax(0, 1fr));
      }
      .insights-grid {
        grid-template-columns: 1fr;
      }
      .insight-wide {
        grid-column: span 1;
      }
      .compact-meta {
        grid-template-columns: 1fr;
      }
    }
  </style>
</head>
<body>
  <div class="page">
    <header class="topbar">
      <div class="title-block">
        <h1>AgenLeash Stats</h1>
        <p>Agent inventory, cache snapshot, adapter distribution and workspace hotspots.</p>
      </div>
      <div class="topbar-actions">
        <span id="generatedAtTop" class="generated-pill">Not loaded</span>
        <button id="refreshTop" type="button">Refresh</button>
      </div>
    </header>

    <section class="card controls-card">
      <div class="control-grid">
        <div class="field">
          <label for="token">AGENLEASH_TOKEN</label>
          <input id="token" type="password" placeholder="Enter a token to load the dashboard">
        </div>
        <button id="load" class="primary" type="button">Load Dashboard</button>
        <button id="clear" type="button">Clear Token</button>
        <span class="pill pill-accent">Stats Console</span>
      </div>
      <div id="status" class="status">Waiting for a token.</div>
    </section>

    <div id="emptyState" class="empty">
      <div class="empty-card">
        <h2>Ready To Inspect</h2>
        <p>After you enter a token, this page shows the current AgenLeash cache, agent inventory, and workspace hotspots. The layout now treats the agent list as the primary admin table instead of a long card feed.</p>
      </div>
    </div>

    <div id="content" hidden>
      <section id="totals" class="summary-grid"></section>

      <section class="card">
        <div class="section-head">
          <div>
            <h2 class="section-title">Agent List</h2>
            <p class="section-copy">Sorted by current priority, with filters for adapter, state, origin, and free-text search plus pagination.</p>
          </div>
          <span id="agentCount" class="pill">0 shown</span>
        </div>

        <div class="filters-grid">
          <div class="field">
            <label for="agentFilter">Search</label>
            <input id="agentFilter" type="search" placeholder="Search adapter / workspace / session id / native id / preview">
          </div>
          <div class="field">
            <label for="adapterFilter">Adapter</label>
            <select id="adapterFilter">
              <option value="">All adapters</option>
            </select>
          </div>
          <div class="field">
            <label for="stateFilter">State</label>
            <select id="stateFilter">
              <option value="">All states</option>
              <option value="pending">pending</option>
              <option value="running">running</option>
              <option value="paused">paused</option>
              <option value="stopped">stopped</option>
              <option value="errored">errored</option>
            </select>
          </div>
          <div class="field">
            <label for="originFilter">Origin</label>
            <select id="originFilter">
              <option value="">All origins</option>
              <option value="managed">managed</option>
              <option value="discovered">discovered</option>
            </select>
          </div>
          <div class="field">
            <label for="pageSize">Rows</label>
            <select id="pageSize">
              <option value="10">10</option>
              <option value="20" selected>20</option>
              <option value="50">50</option>
              <option value="100">100</option>
            </select>
          </div>
        </div>

        <div class="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Agent</th>
                <th>State</th>
                <th>Origin</th>
                <th>Session</th>
                <th>Workspace</th>
                <th>Last Seen</th>
                <th>Preview</th>
              </tr>
            </thead>
            <tbody id="agentsBody"></tbody>
          </table>
        </div>

        <div class="pagination">
          <div id="pageInfo" class="pagination-info">0 results</div>
          <div class="pagination-controls">
            <button id="prevPage" type="button">Prev</button>
            <span id="pageLabel" class="pill">Page 1 / 1</span>
            <button id="nextPage" type="button">Next</button>
          </div>
        </div>
      </section>

      <div class="insights-grid">
        <section class="card">
          <div class="section-head">
            <div>
              <h2 class="section-title">Machine Snapshot</h2>
              <p class="section-copy">Current cache overview, key counters, and recent activity.</p>
            </div>
            <span id="summaryBadge" class="pill">0 sessions</span>
          </div>
          <div id="summary" class="compact-meta"></div>
        </section>

        <section class="card">
          <div class="section-head">
            <div>
              <h2 class="section-title">Adapters</h2>
              <p class="section-copy">Session totals, sources, and running state grouped by adapter.</p>
            </div>
          </div>
          <div class="mini-table-wrap">
            <table class="mini-table">
              <thead>
                <tr>
                  <th>Adapter</th>
                  <th>Sessions</th>
                  <th>Managed</th>
                  <th>Running</th>
                </tr>
              </thead>
              <tbody id="adapters"></tbody>
            </table>
          </div>
        </section>

        <section class="card insight-wide">
          <div class="section-head">
            <div>
              <h2 class="section-title">Top Workspaces</h2>
              <p class="section-copy">A compact hotspot table with the workspace stats you are most likely to check.</p>
            </div>
          </div>
          <div class="mini-table-wrap">
            <table class="mini-table">
              <thead>
                <tr>
                  <th>Workspace</th>
                  <th>Sessions</th>
                  <th>Adapters</th>
                  <th>Last Seen</th>
                </tr>
              </thead>
              <tbody id="workspaces"></tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  </div>

  <script>
    const STORAGE_KEY = "agenleash.stats.token";
    const tokenInput = document.getElementById("token");
    const statusEl = document.getElementById("status");
    const emptyStateEl = document.getElementById("emptyState");
    const contentEl = document.getElementById("content");
    const totalsEl = document.getElementById("totals");
    const summaryEl = document.getElementById("summary");
    const summaryBadgeEl = document.getElementById("summaryBadge");
    const generatedAtTopEl = document.getElementById("generatedAtTop");
    const adaptersEl = document.getElementById("adapters");
    const workspacesEl = document.getElementById("workspaces");
    const agentsBodyEl = document.getElementById("agentsBody");
    const agentCountEl = document.getElementById("agentCount");
    const pageInfoEl = document.getElementById("pageInfo");
    const pageLabelEl = document.getElementById("pageLabel");
    const prevPageBtn = document.getElementById("prevPage");
    const nextPageBtn = document.getElementById("nextPage");
    const agentFilterInput = document.getElementById("agentFilter");
    const adapterFilterEl = document.getElementById("adapterFilter");
    const stateFilterEl = document.getElementById("stateFilter");
    const originFilterEl = document.getElementById("originFilter");
    const pageSizeEl = document.getElementById("pageSize");

    let latestSessions = [];
    let currentPage = 1;

    function setStatus(message, tone) {
      statusEl.textContent = message;
      statusEl.className = "status" + (tone ? " " + tone : "");
    }

    function fmtNumber(value) {
      return new Intl.NumberFormat("en-US").format(value || 0);
    }

    function fmtTime(value) {
      if (!value) return "—";
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return "—";
      return date.toLocaleString("en-US", { hour12: false });
    }

    function escapeHTML(text) {
      return String(text || "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
    }

    function firstNonEmpty() {
      for (let i = 0; i < arguments.length; i += 1) {
        const value = String(arguments[i] || "").trim();
        if (value) return value;
      }
      return "";
    }

    function basename(path) {
      const value = String(path || "").trim();
      if (!value) return "unknown";
      const parts = value.split("/").filter(Boolean);
      return parts.length ? parts[parts.length - 1] : "/";
    }

    function shortText(text, limit) {
      const value = String(text || "").trim();
      if (!value) return "—";
      if (value.length <= limit) return value;
      return value.slice(0, limit - 1) + "…";
    }

    function shortID(value) {
      const text = String(value || "").trim();
      if (!text) return "—";
      if (text.length <= 18) return text;
      return text.slice(0, 8) + "…" + text.slice(-8);
    }

    function pillClassFromState(state) {
      switch (String(state || "").toLowerCase()) {
        case "running":
          return "pill-success";
        case "paused":
        case "pending":
          return "pill-warn";
        case "errored":
          return "pill-danger";
        default:
          return "";
      }
    }

    function pillClassFromTone(tone) {
      switch (String(tone || "").toLowerCase()) {
        case "success":
          return "pill-success";
        case "warning":
          return "pill-warn";
        case "danger":
          return "pill-danger";
        case "info":
          return "pill-info";
        default:
          return "";
      }
    }

    function metric(label, value, hint) {
      return "<div class='metric-card'>" +
        "<div class='label'>" + escapeHTML(label) + "</div>" +
        "<div class='value'>" + fmtNumber(value) + "</div>" +
        "<div class='hint'>" + escapeHTML(hint || " ") + "</div>" +
      "</div>";
    }

    function metaChip(label, value) {
      return "<div class='meta-chip'>" +
        "<strong>" + escapeHTML(label) + "</strong>" +
        "<span>" + escapeHTML(value) + "</span>" +
      "</div>";
    }

    function countAdapter(items, name) {
      for (const item of items || []) {
        if (item.adapter === name) return item.sessions || 0;
      }
      return 0;
    }

    function populateAdapterFilter() {
      const current = adapterFilterEl.value;
      const names = Array.from(new Set(latestSessions.map(function(session) {
        return String(session.adapter || "").trim();
      }).filter(Boolean))).sort();

      let html = "<option value=''>All adapters</option>";
      for (const name of names) {
        html += "<option value='" + escapeHTML(name) + "'>" + escapeHTML(name) + "</option>";
      }
      adapterFilterEl.innerHTML = html;
      adapterFilterEl.value = current && names.includes(current) ? current : "";
    }

    function renderTotals(data) {
      const totals = data.totals || {};
      totalsEl.innerHTML =
        metric("Sessions", totals.sessions, "All sessions currently in cache") +
        metric("Managed", totals.managed, "Sessions actively managed by AgenLeash") +
        metric("Discovered", totals.discovered, "Sessions discovered from local history") +
        metric("ClaudeCode", (data.claudecode && data.claudecode.sessions) || 0, "Primary sessions plus subagents") +
        metric("Codex", countAdapter(data.adapters, "codex"), "Codex history and managed sessions") +
        metric("OpenCode", countAdapter(data.adapters, "opencode"), "OpenCode history and managed sessions");
    }

    function renderSummary(data) {
      const totals = data.totals || {};
      summaryBadgeEl.textContent = fmtNumber(totals.sessions) + " sessions";
      generatedAtTopEl.textContent = fmtTime(data.generated_at);
      summaryEl.innerHTML =
        metaChip("Generated", fmtTime(data.generated_at)) +
        metaChip("Managed / Discovered", fmtNumber(totals.managed) + " / " + fmtNumber(totals.discovered)) +
        metaChip("Recent Activity", fmtNumber(totals.recent_activity)) +
        metaChip("Review Required", fmtNumber(totals.review_required)) +
        metaChip("Unique Workspaces", fmtNumber(totals.unique_workspaces)) +
        metaChip("Unique Conversations", fmtNumber(totals.unique_conversations));
    }

    function renderAdapters(data) {
      const items = data.adapters || [];
      if (!items.length) {
        adaptersEl.innerHTML = "<tr><td colspan='4'><div class='empty-inline'>No adapter data available.</div></td></tr>";
        return;
      }

      let html = "";
      for (const item of items) {
        html += "<tr>" +
          "<td>" +
            "<div class='mini-title'>" + escapeHTML(item.adapter || "unknown") + "</div>" +
            "<div class='subtle-line'>Last seen · " + escapeHTML(fmtTime(item.last_seen)) + "</div>" +
          "</td>" +
          "<td>" + escapeHTML(fmtNumber(item.sessions)) + "</td>" +
          "<td>" + escapeHTML(fmtNumber(item.managed)) + "</td>" +
          "<td>" + escapeHTML(fmtNumber(item.running)) + "</td>" +
        "</tr>";
      }
      adaptersEl.innerHTML = html;
    }

    function renderWorkspaces(data) {
      const items = data.top_workspaces || [];
      if (!items.length) {
        workspacesEl.innerHTML = "<tr><td colspan='4'><div class='empty-inline'>No workspace stats available.</div></td></tr>";
        return;
      }

      let html = "";
      for (const item of items.slice(0, 8)) {
        html += "<tr>" +
          "<td>" +
            "<div class='mini-title'>" + escapeHTML(basename(item.path)) + "</div>" +
            "<div class='subtle-line'>" + escapeHTML(item.path) + "</div>" +
          "</td>" +
          "<td>" + escapeHTML(fmtNumber(item.sessions)) + "</td>" +
          "<td>" + escapeHTML((item.adapters || []).join(", ") || "—") + "</td>" +
          "<td>" + escapeHTML(fmtTime(item.last_seen)) + "</td>" +
        "</tr>";
      }
      workspacesEl.innerHTML = html;
    }

    function sessionSearchText(session) {
      return [
        session.id,
        session.adapter,
        session.origin,
        session.state,
        session.conversation && session.conversation.native_id,
        session.workspace && session.workspace.cwd,
        session.workspace && session.workspace.root,
        session.last_output_preview,
        session.highlight && session.highlight.label,
        session.highlight && session.highlight.detail
      ].join(" ").toLowerCase();
    }

    function filteredSessions() {
      const query = String(agentFilterInput.value || "").trim().toLowerCase();
      const adapter = String(adapterFilterEl.value || "").trim().toLowerCase();
      const state = String(stateFilterEl.value || "").trim().toLowerCase();
      const origin = String(originFilterEl.value || "").trim().toLowerCase();

      return latestSessions.filter(function(session) {
        if (adapter && String(session.adapter || "").toLowerCase() !== adapter) return false;
        if (state && String(session.state || "").toLowerCase() !== state) return false;
        if (origin && String(session.origin || "").toLowerCase() !== origin) return false;
        if (query && !sessionSearchText(session).includes(query)) return false;
        return true;
      });
    }

    function renderAgents() {
      const sessions = filteredSessions();
      const pageSize = Math.max(1, parseInt(pageSizeEl.value, 10) || 20);
      const total = sessions.length;
      const pageCount = Math.max(1, Math.ceil(total / pageSize));
      currentPage = Math.min(Math.max(currentPage, 1), pageCount);

      const start = total === 0 ? 0 : (currentPage - 1) * pageSize;
      const end = Math.min(start + pageSize, total);
      const pageItems = sessions.slice(start, end);

      agentCountEl.textContent = fmtNumber(total) + " agents";
      pageInfoEl.textContent = total === 0
        ? "0 results"
        : "Showing " + fmtNumber(start + 1) + "-" + fmtNumber(end) + " of " + fmtNumber(total);
      pageLabelEl.textContent = "Page " + currentPage + " / " + pageCount;
      prevPageBtn.disabled = currentPage <= 1;
      nextPageBtn.disabled = currentPage >= pageCount;

      if (!pageItems.length) {
        agentsBodyEl.innerHTML = "<tr><td colspan='7'><div class='empty-inline'>No agents match the current filters.</div></td></tr>";
        return;
      }

      let html = "";
      for (const session of pageItems) {
        const workspace = firstNonEmpty(session.workspace && session.workspace.cwd, session.workspace && session.workspace.root);
        const nativeID = session.conversation && session.conversation.native_id;
        const preview = firstNonEmpty(session.last_output_preview, session.highlight && session.highlight.detail, workspace);
        const stateClass = pillClassFromState(session.state);
        const highlightClass = pillClassFromTone(session.highlight && session.highlight.tone);

        html += "<tr>" +
          "<td>" +
            "<div class='agent-main'>" +
              "<div class='agent-title'>" +
                "<strong>" + escapeHTML(firstNonEmpty(session.adapter, "unknown")) + " · " + escapeHTML(basename(workspace || session.id)) + "</strong>" +
                (session.highlight && session.highlight.label ? "<span class='pill " + highlightClass + "'>" + escapeHTML(session.highlight.label) + "</span>" : "") +
              "</div>" +
              "<div class='agent-subtle'>" + escapeHTML(firstNonEmpty(workspace, "—")) + "</div>" +
            "</div>" +
          "</td>" +
          "<td><span class='pill " + stateClass + "'>" + escapeHTML(firstNonEmpty(session.state, "unknown")) + "</span></td>" +
          "<td><span class='pill'>" + escapeHTML(firstNonEmpty(session.origin, "managed")) + "</span></td>" +
          "<td>" +
            "<div class='mono'>" + escapeHTML(shortID(session.id)) + "</div>" +
            "<div class='agent-subtle'>" + escapeHTML(shortID(nativeID)) + "</div>" +
          "</td>" +
          "<td>" +
            "<div>" + escapeHTML(basename(workspace)) + "</div>" +
            "<div class='agent-subtle'>" + escapeHTML(shortText(workspace, 44)) + "</div>" +
          "</td>" +
          "<td>" +
            "<div>" + escapeHTML(fmtTime(session.last_seen)) + "</div>" +
            "<div class='agent-subtle'>" + escapeHTML(firstNonEmpty(session.conversation && session.conversation.start_mode, "—")) + "</div>" +
          "</td>" +
          "<td><div class='preview'>" + escapeHTML(preview) + "</div></td>" +
        "</tr>";
      }
      agentsBodyEl.innerHTML = html;
    }

    function resetPageAndRenderAgents() {
      currentPage = 1;
      renderAgents();
    }

    function showLoadedState() {
      contentEl.hidden = false;
      emptyStateEl.hidden = true;
    }

    function showEmptyState() {
      contentEl.hidden = true;
      emptyStateEl.hidden = false;
      latestSessions = [];
      currentPage = 1;
      totalsEl.innerHTML = "";
      summaryEl.innerHTML = "";
      adaptersEl.innerHTML = "";
      workspacesEl.innerHTML = "";
      agentsBodyEl.innerHTML = "";
      summaryBadgeEl.textContent = "0 sessions";
      generatedAtTopEl.textContent = "Not loaded";
      agentCountEl.textContent = "0 agents";
      pageInfoEl.textContent = "0 results";
      pageLabelEl.textContent = "Page 1 / 1";
    }

    async function loadDashboard() {
      const token = tokenInput.value.trim();
      if (!token) {
        showEmptyState();
        setStatus("Enter a token before loading the dashboard.", "error");
        return;
      }

      localStorage.setItem(STORAGE_KEY, token);
      setStatus("Loading stats and agent list...", "loading");

      try {
        const headers = { "X-AgenLeash-Token": token };
        const responses = await Promise.all([
          fetch("/api/v1/stats?top=8", { cache: "no-store", headers: headers }),
          fetch("/api/v1/sessions?limit=1000", { cache: "no-store", headers: headers })
        ]);
        for (const response of responses) {
          if (!response.ok) {
            const body = await response.json().catch(function() { return {}; });
            throw new Error(body.error || ("HTTP " + response.status));
          }
        }

        const statsData = await responses[0].json();
        const sessionData = await responses[1].json();
        latestSessions = Array.isArray(sessionData.sessions) ? sessionData.sessions : [];

        populateAdapterFilter();
        renderTotals(statsData);
        renderSummary(statsData);
        renderAdapters(statsData);
        renderWorkspaces(statsData);
        currentPage = 1;
        renderAgents();
        showLoadedState();
        setStatus("Dashboard updated · " + fmtTime(statsData.generated_at), "success");
      } catch (error) {
        showEmptyState();
        setStatus("Load failed: " + error.message, "error");
      }
    }

    document.getElementById("load").addEventListener("click", loadDashboard);
    document.getElementById("refreshTop").addEventListener("click", loadDashboard);
    document.getElementById("clear").addEventListener("click", function() {
      localStorage.removeItem(STORAGE_KEY);
      tokenInput.value = "";
      agentFilterInput.value = "";
      adapterFilterEl.value = "";
      stateFilterEl.value = "";
      originFilterEl.value = "";
      pageSizeEl.value = "20";
      showEmptyState();
      setStatus("Token cleared.", "");
    });
    agentFilterInput.addEventListener("input", resetPageAndRenderAgents);
    adapterFilterEl.addEventListener("change", resetPageAndRenderAgents);
    stateFilterEl.addEventListener("change", resetPageAndRenderAgents);
    originFilterEl.addEventListener("change", resetPageAndRenderAgents);
    pageSizeEl.addEventListener("change", resetPageAndRenderAgents);
    prevPageBtn.addEventListener("click", function() {
      currentPage = Math.max(1, currentPage - 1);
      renderAgents();
    });
    nextPageBtn.addEventListener("click", function() {
      currentPage += 1;
      renderAgents();
    });

    window.addEventListener("keydown", function(event) {
      if (event.key === "Enter" && document.activeElement === tokenInput) {
        loadDashboard();
      }
    });

    const savedToken = new URLSearchParams(window.location.search).get("token") || localStorage.getItem(STORAGE_KEY) || "";
    if (savedToken) {
      tokenInput.value = savedToken;
      loadDashboard();
    } else {
      showEmptyState();
    }
  </script>
</body>
</html>
`

func (r *Router) handleStatsPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(statsPageHTML))
}
