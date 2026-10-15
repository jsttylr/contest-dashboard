'use strict';
// Helpers shared by every dashboard page.

const params = new URLSearchParams(location.search);
let contest = params.get('contest') || '';

const BANDS = ['160m', '80m', '40m', '20m', '15m', '10m', '6m'];
function bandVar(b) { return BANDS.indexOf(b) < 0 ? '--bother' : '--b' + b.replace('m', ''); }
function css(v) { return getComputedStyle(document.documentElement).getPropertyValue(v).trim(); }
function bandColor(b) { return css(bandVar(b)); }
// Categorical colours for things that aren't bands (operators, run vs S&P), in a fixed order.
const SERIES = ['--s1', '--s2', '--s3', '--s4', '--s5', '--s6', '--s7', '--s8'];
function seriesColor(i) { return css(SERIES[i % SERIES.length]); }
function el(id) { return document.getElementById(id); }
function esc(s) { return String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }
function fmt(n) { return Number(n || 0).toLocaleString(); }
function fmt1(n) { return Number(n || 0).toLocaleString(undefined, { maximumFractionDigits: 1 }); }
function hhmm(t) { return new Date(t).toISOString().slice(11, 16); }
function hours(h) { const m = Math.round((h || 0) * 60); return m < 60 ? `${m}m` : `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, '0')}m`; }
function modeName(m) { return { CW: 'CW', PH: 'Phone', DG: 'Digital' }[m] || m || '?'; }
// q adds the chosen contest to a page or API link.
function q(path, extra) {
  const u = new URLSearchParams(extra || {});
  if (contest) u.set('contest', contest);
  const s = u.toString();
  return s ? path + (path.includes('?') ? '&' : '?') + s : path;
}

if (!document.documentElement.dataset.theme) document.documentElement.dataset.theme = 'dark';
function onThemeToggle(after) {
  const b = el('theme');
  if (!b) return;
  b.onclick = () => {
    const next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem('theme', next); } catch (e) { }
    after && after();
  };
}

function startClock() {
  const c = el('clock');
  if (c) setInterval(() => { c.textContent = new Date().toISOString().slice(11, 19) + ' UTC'; }, 1000);
}

function chartDefaults() {
  if (!window.Chart) return;
  Chart.defaults.color = css('--text-secondary');
  Chart.defaults.borderColor = css('--border');
  Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
}

function liveBadge(active, live) {
  el('live').classList.toggle('live', !!(active && live));
  el('live-text').textContent = !active ? 'Waiting for contacts' : live ? 'Live' : 'Finished';
}

// Calls refresh now and whenever the server says something changed (at most once a second).
function followUpdates(refresh) {
  let loading = false, again = false;
  const run = async () => {
    if (loading) { again = true; return; }
    loading = true;
    try { await refresh(); } catch (e) {
      if (el('live-text')) { el('live-text').textContent = 'Not connected'; el('live').classList.remove('live'); }
    } finally {
      loading = false;
      if (again) { again = false; run(); }
    }
  };
  run();
  try {
    const es = new EventSource('api/events');
    es.onmessage = run;
    es.onerror = () => { if (el('live-text')) el('live-text').textContent = 'Reconnecting…'; };
  } catch (e) { setInterval(run, 5000); }
  return run;
}

// A small table that sorts when a column heading is clicked.
function sortableTable(table, columns, rows, state, rowHtml) {
  const key = state.key, dir = state.dir;
  const col = columns.find(c => c.key === key) || columns[0];
  const sorted = rows.slice().sort((a, b) => {
    const x = col.value(a), y = col.value(b);
    return (typeof x === 'string' ? x.localeCompare(y) : x - y) * dir;
  });
  // Mark the best value in each numeric column.
  const best = {};
  columns.forEach(c => { if (c.best) best[c.key] = Math.max(...rows.map(c.value)); });
  table.innerHTML = '<tr>' + columns.map(c => `<th class="${c.num ? 'r' : ''} sort" data-k="${c.key}" title="${esc(c.help || 'Sort')}">${esc(c.label)}${c.key === key ? (dir < 0 ? ' ▾' : ' ▴') : ''}</th>`).join('') + '</tr>' +
    sorted.map(r => rowHtml(r, (c, text) => {
      const isBest = c.best && rows.length > 1 && c.value(r) === best[c.key] && best[c.key] > 0;
      return `<td class="${c.num ? 'r' : ''}${isBest ? ' best' : ''}">${text}</td>`;
    })).join('');
  table.querySelectorAll('th.sort').forEach(th => th.onclick = () => {
    const k = th.dataset.k;
    state.dir = state.key === k ? -state.dir : (columns.find(c => c.key === k).num ? -1 : 1);
    state.key = k;
    sortableTable(table, columns, rows, state, rowHtml);
  });
}

// ---- tables and charts used on more than one page ----

function tile(label, value, hint, hero) {
  return `<div class="tile${hero ? ' hero' : ''}"><div class="label">${esc(label)}</div><div class="value">${value}</div>${hint ? `<div class="hint">${hint}</div>` : '<div class="hint">&nbsp;</div>'}</div>`;
}


function bandModeTable(table, bands, modes, bandMode, mults) {
  let h = '<tr><th>Band</th>' + modes.map(m => `<th class="r">${esc(modeName(m))}</th>`).join('') + '<th class="r">QSOs</th>' + (mults ? '<th class="r">Mults</th>' : '') + '</tr>';
  const colTot = {};
  let all = 0;
  bands.forEach(b => {
    let tot = 0;
    h += `<tr><td><span class="band-dot" style="background:${bandColor(b)}"></span>${esc(b || '?')}</td>` + modes.map(m => {
      const n = (bandMode[b] || {})[m] || 0; tot += n; colTot[m] = (colTot[m] || 0) + n;
      return `<td class="r">${n ? fmt(n) : '<span class="muted">·</span>'}</td>`;
    }).join('') + `<td class="r"><b>${fmt(tot)}</b></td>` + (mults ? `<td class="r">${fmt(mults[b] || 0)}</td>` : '') + '</tr>';
    all += tot;
  });
  if (bands.length) h += '<tr><td><b>Total</b></td>' + modes.map(m => `<td class="r"><b>${fmt(colTot[m])}</b></td>`).join('') + `<td class="r"><b>${fmt(all)}</b></td>` + (mults ? '<td></td>' : '') + '</tr>';
  table.innerHTML = h;
}

function hourlyTable(table, hourly, bands) {
  hourly = hourly || [];
  if (!hourly.length) { table.innerHTML = ''; return; }
  const max = Math.max(1, ...hourly.map(h => h.total));
  const accent = css('--accent');
  const cell = n => n
    ? `<td class="c" style="background:color-mix(in srgb, ${accent} ${Math.round(15 + 70 * n / max)}%, transparent)">${n}</td>`
    : '<td class="c muted">·</td>';
  let h = '<tr><th>Hour (UTC)</th>' + bands.map(b => `<th class="r">${esc(b)}</th>`).join('') + '<th class="r">Total</th><th class="r">Running total</th></tr>';
  let run = 0;
  hourly.forEach(x => {
    run += x.total;
    const d = new Date(x.t).toISOString();
    h += `<tr><td>${d.slice(5, 10)} ${d.slice(11, 13)}z</td>` + bands.map(b => cell(x.by_band[b] || 0)).join('') + cell(x.total) + `<td class="r muted">${fmt(run)}</td></tr>`;
  });
  table.innerHTML = h;
}

function recentTable(table, recent, units, showOp) {
  const unit = units === 'km' ? 'km' : 'mi';
  const dist = d => d ? fmt(Math.round(unit === 'km' ? d : d / 1.609344)) : '';
  table.innerHTML = `<tr><th>UTC</th><th>Call</th><th>Band</th>${showOp ? '<th>Op</th>' : ''}<th>Country</th><th class="r">${unit}</th><th></th></tr>` +
    (recent || []).map(r => `<tr><td class="muted">${hhmm(r.time)}</td><td class="call">${esc(r.call)}</td>
      <td><span class="band-dot" style="background:${bandColor(r.band)}"></span>${esc(r.band)} ${esc(r.mode)}</td>
      ${showOp ? `<td><a class="oplink" href="${q('operator.html', { op: r.op })}">${esc(r.op)}</a></td>` : ''}<td>${esc(r.country)}</td><td class="r">${dist(r.dist)}</td>
      <td>${r.mult ? '<span class="tag mult">MULT</span> ' : ''}${r.run ? '<span class="tag run">RUN</span>' : ''}</td></tr>`).join('');
}

// Draws or updates a chart in place; returns the chart.
function upsertChart(chart, canvas, config) {
  chartDefaults();
  if (!chart) return new Chart(canvas, config);
  chart.data = config.data;
  Object.assign(chart.options.scales || {}, config.options.scales || {});
  chart.update('none');
  return chart;
}

function countryBars(chart, canvas, countries) {
  const tc = countries || [];
  return upsertChart(chart, canvas, {
    type: 'bar',
    data: { labels: tc.map(c => c.name), datasets: [{ label: 'QSOs', data: tc.map(c => c.qsos), backgroundColor: css('--accent'), borderRadius: 4, borderSkipped: 'start', barThickness: 14 }] },
    options: {
      indexAxis: 'y', animation: false, maintainAspectRatio: false,
      scales: { x: { beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } }, y: { grid: { display: false } } },
      plugins: { legend: { display: false } }
    }
  });
}

const legendOpts = { position: 'top', align: 'end', labels: { boxWidth: 10, boxHeight: 10, usePointStyle: true, pointStyle: 'circle' } };
