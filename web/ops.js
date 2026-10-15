'use strict';
// Operator comparison page.

let ratesChart = null, cumulChart = null, rollingChart = null, lastData = null;
let rateWindow = 30, rateKind = 'all';
const sortState = { key: 'qsos', dir: -1 };

const columns = [
  { key: 'name', label: 'Operator', value: o => o.name },
  { key: 'qsos', label: 'QSOs', num: true, best: true, value: o => o.qsos },
  { key: 'share', label: 'Share', num: true, value: o => o.share, help: "Share of the station's QSOs" },
  { key: 'points', label: 'Points', num: true, best: true, value: o => o.points },
  { key: 'mults', label: 'Mults', num: true, best: true, value: o => o.mults, help: 'Contacts that were new multipliers' },
  { key: 'run_rate', label: 'Run rate', num: true, best: true, value: o => o.run_rate, help: 'QSOs per hour while running' },
  { key: 'sp_rate', label: 'S&P rate', num: true, best: true, value: o => o.sp_rate, help: 'QSOs per hour while searching and pouncing' },
  { key: 'avg_rate', label: 'Avg rate', num: true, best: true, value: o => o.avg_rate, help: 'QSOs per hour on the air' },
  { key: 'best60', label: 'Best hour', num: true, best: true, value: o => o.best60, help: 'Most QSOs in any 60 minutes' },
  { key: 'best10', label: 'Best 10 min', num: true, best: true, value: o => o.best10, help: 'Most QSOs in any 10 minutes, shown as an hourly rate' },
  { key: 'run_pct', label: 'Run %', num: true, value: o => o.qsos ? 100 * o.run_qsos / o.qsos : 0, help: 'Share of QSOs made while running' },
  { key: 'on_hours', label: 'On air', num: true, value: o => o.on_hours, help: 'Time on the air, breaks of 30 minutes or more left out' },
  { key: 'last60', label: 'Last hour', num: true, best: true, value: o => o.last60 },
  { key: 'countries', label: 'Countries', num: true, best: true, value: o => o.countries },
];

function cellText(c, o) {
  switch (c.key) {
    case 'name': return `<a class="oplink call" href="${q('operator.html', { op: o.name })}">${esc(o.name)}</a>`;
    case 'share': case 'run_pct': return Math.round(c.value(o)) + '%';
    case 'run_rate': case 'sp_rate': case 'avg_rate': return fmt(Math.round(c.value(o)));
    case 'best10': return fmt(o.best10 * 6);
    case 'on_hours': return hours(o.on_hours);
    default: return fmt(c.value(o));
  }
}

// Each operator keeps one colour however the list is sorted: by name order.
function opColors(ops) {
  const names = ops.map(o => o.name).sort();
  return name => seriesColor(names.indexOf(name));
}

function render(d) {
  lastData = d;
  liveBadge(d.active, d.active && Date.now() - new Date(d.active.last) < 6 * 3600e3);
  el('contest-name').textContent = d.active ? d.active.name : '';
  const ops = d.operators || [];
  sortableTable(el('compare'), columns, ops, sortState, (o, td) => '<tr>' + columns.map(c => td(c, cellText(c, o))).join('') + '</tr>');

  renderRolling(d);
  el('maxrates').innerHTML = '<tr><th>Operator</th>' + maxRateHeads.map(h => `<th class="r">${esc(h)}</th>`).join('') + '</tr>' +
    ops.concat(ops.length > 1 ? [{ name: 'Whole station', max: d.station, station: true }] : []).map(o => '<tr>' +
      (o.station ? '<td><b>Whole station</b></td>' : `<td><a class="oplink call" href="${q('operator.html', { op: o.name })}">${esc(o.name)}</a></td>`) +
      maxRateCells(o.max || {}).map(c => `<td class="r">${c}</td>`).join('') + '</tr>').join('');

  const s1 = seriesColor(0), s2 = seriesColor(1);
  ratesChart = upsertChart(ratesChart, el('rates-chart'), {
    type: 'bar',
    data: {
      labels: ops.map(o => o.name),
      datasets: [
        { label: 'Run', data: ops.map(o => Math.round(o.run_rate)), backgroundColor: s1, borderRadius: 4, borderSkipped: 'start', barPercentage: 0.8, categoryPercentage: 0.7 },
        { label: 'S&P', data: ops.map(o => Math.round(o.sp_rate)), backgroundColor: s2, borderRadius: 4, borderSkipped: 'start', barPercentage: 0.8, categoryPercentage: 0.7 },
      ]
    },
    options: {
      indexAxis: 'y', animation: false, maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: { x: { beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } }, y: { grid: { display: false } } },
      plugins: { legend: legendOpts, tooltip: { callbacks: { label: i => `${i.dataset.label}: ${i.parsed.x} QSOs/hour` } } }
    }
  });

  // Running totals: the eight busiest operators, so colours stay distinct.
  const shown = ops.slice(0, 8);
  const color = opColors(shown);
  el('cumul-note').textContent = 'running total of QSOs' + (ops.length > 8 ? `, busiest 8 of ${ops.length} operators` : '');
  cumulChart = upsertChart(cumulChart, el('cumul-chart'), {
    type: 'line',
    data: {
      labels: (d.slots || []).map(t => hhmm(t)),
      datasets: shown.map(o => ({
        label: o.name, data: d.cumulative[o.name], borderColor: color(o.name), backgroundColor: color(o.name),
        borderWidth: 2, pointRadius: 0, pointHoverRadius: 4, tension: 0.2
      }))
    },
    options: {
      animation: false, maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: { x: { grid: { display: false }, ticks: { autoSkip: true, maxRotation: 0 } }, y: { beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } } },
      plugins: { legend: legendOpts }
    }
  });
}

function renderRolling(d) {
  const ops = (d.operators || []).slice(0, 8);
  if (!ops.length || !d.active) return;
  const first = new Date(d.active.first) / 1000, last = new Date(d.active.last) / 1000;
  const step = stepFor(first, last);
  const color = opColors(ops);
  let labels = [];
  const datasets = ops.map(o => {
    const r = rollingRates(d.points[o.name] || [], first, last, rateWindow, step);
    labels = r.t.map(hhmm);
    return { label: o.name, data: rateKind === 'run' ? r.run : r.all, borderColor: color(o.name), backgroundColor: color(o.name), borderWidth: 2, pointRadius: 0, pointHoverRadius: 4, tension: 0.25 };
  });
  el('rolling-note').textContent = (rateKind === 'run' ? 'QSOs per hour while running' : 'QSOs per hour') + `, over the last ${rateWindow} minutes at each point; gaps are time off the air` + ((d.operators || []).length > 8 ? '; busiest 8 operators' : '');
  rollingChart = upsertChart(rollingChart, el('rolling-chart'), { type: 'line', data: { labels, datasets }, options: lineOptions('QSOs per hour') });
}

onThemeToggle(() => { [ratesChart, cumulChart, rollingChart].forEach(c => c?.destroy()); ratesChart = cumulChart = rollingChart = null; if (lastData) render(lastData); });
choiceButtons(el('rate-kind'), [['all', 'All contacts'], ['run', 'Running only']], rateKind, v => { rateKind = v; if (lastData) renderRolling(lastData); });
choiceButtons(el('rate-window'), [[15, '15 min'], [30, '30 min'], [60, '60 min']], rateWindow, v => { rateWindow = v; if (lastData) renderRolling(lastData); });
startClock();
el('dash-link').href = q('./');
followUpdates(async () => render(await (await fetch(q('api/operators'))).json()));
