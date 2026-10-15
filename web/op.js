'use strict';
// One operator's details.

const op = (params.get('op') || '').toUpperCase();
let runspChart = null, countryChart = null, rollingChart = null, lastData = null;
let rateWindow = 30;

function render(d) {
  lastData = d;
  const s = d.stats;
  el('op-name').textContent = op || 'Operator';
  document.title = `${op} · Operator`;
  el('contest-name').textContent = (d.active ? d.active.name : '') + ((s.stations || []).length ? ' · at ' + s.stations.join(', ') : '');
  liveBadge(d.active, d.active && Date.now() - new Date(d.active.last) < 6 * 3600e3);
  el('empty').hidden = s.qsos > 0;

  el('tiles').innerHTML =
    tile('QSOs', fmt(s.qsos), `${Math.round(s.share)}% of the station's`, true) +
    tile('Points', fmt(s.points)) +
    tile('Mults', fmt(s.mults), 'new multipliers worked') +
    tile('Run rate', fmt(Math.round(s.run_rate)), `${fmt(s.run_qsos)} QSOs in ${hours(s.run_hours)} running`) +
    tile('S&P rate', fmt(Math.round(s.sp_rate)), `${fmt(s.sp_qsos)} QSOs in ${hours(s.sp_hours)} S&P`) +
    tile('Average rate', fmt(Math.round(s.avg_rate)), `over ${hours(s.on_hours)} on the air`) +
    tile('Best hour', fmt(s.best60), 'any 60 minutes') +
    tile('Best 10 min', fmt(s.best10 * 6), `${s.best10} QSOs, as an hourly rate`) +
    tile('Last hour', fmt(s.last60)) +
    tile('Countries', fmt(s.countries));

  const tl = d.timeline || [];
  el('rate-note').textContent = d.bucket_min ? `QSOs per ${d.bucket_min} minutes, running or S&P` : '';
  const surface = css('--surface-1');
  runspChart = upsertChart(runspChart, el('runsp-chart'), {
    type: 'bar',
    data: {
      labels: tl.map(b => hhmm(b.t)),
      datasets: [
        { label: 'Run', data: tl.map(b => b.run), backgroundColor: seriesColor(0), borderColor: surface, borderWidth: { top: 1 }, borderRadius: 2, borderSkipped: 'bottom' },
        { label: 'S&P', data: tl.map(b => b.sp), backgroundColor: seriesColor(1), borderColor: surface, borderWidth: { top: 1 }, borderRadius: 2, borderSkipped: 'bottom' },
      ]
    },
    options: {
      animation: false, maintainAspectRatio: false,
      interaction: { mode: 'index', intersect: false },
      scales: {
        x: { stacked: true, grid: { display: false }, ticks: { autoSkip: true, maxRotation: 0 } },
        y: { stacked: true, beginAtZero: true, ticks: { precision: 0 }, grid: { color: css('--border') } }
      },
      plugins: { legend: legendOpts, tooltip: { filter: i => i.parsed.y > 0 } }
    }
  });

  renderRolling(d);
  el('maxrates').innerHTML = maxRateCells(s.max).map((c, i) => `<tr><th>${esc(maxRateHeads[i])}</th><td class="r">${c}</td></tr>`).join('');
  el('stretches').innerHTML = '<tr><th>Started (UTC)</th><th>Band</th><th class="r">Length</th><th class="r">QSOs</th><th class="r">Rate</th></tr>' +
    ((d.stretches || []).length ? d.stretches.map(x => `<tr><td>${hhmm(x.start)}–${hhmm(x.end)}</td>
      <td><span class="band-dot" style="background:${bandColor(x.band)}"></span>${esc(x.band)}</td><td class="r">${hours(x.minutes / 60)}</td>
      <td class="r">${fmt(x.qsos)}</td><td class="r"><b>${fmt(Math.round(x.rate))}</b>/hr</td></tr>`).join('') : '<tr><td colspan="5" class="muted">No runs yet.</td></tr>');

  bandModeTable(el('bandmode'), d.bands || [], d.modes || [], d.band_mode || {});
  recentTable(el('recent'), d.recent, d.units, false);
  countryChart = countryBars(countryChart, el('country-chart'), d.top_countries);
  hourlyTable(el('hourly'), d.hourly, d.bands || []);
}

function renderRolling(d) {
  const pts = d.points || [];
  if (!pts.length) return;
  const first = new Date(d.contest_first) / 1000, last = new Date(d.contest_last) / 1000;
  const r = rollingRates(pts, first, last, rateWindow, stepFor(first, last));
  rollingChart = upsertChart(rollingChart, el('rolling-chart'), {
    type: 'line',
    data: {
      labels: r.t.map(hhmm),
      datasets: [
        { label: 'Run rate', data: r.run, borderColor: seriesColor(0), backgroundColor: seriesColor(0), borderWidth: 2, pointRadius: 0, pointHoverRadius: 4, tension: 0.25 },
        { label: 'S&P rate', data: r.sp, borderColor: seriesColor(1), backgroundColor: seriesColor(1), borderWidth: 2, pointRadius: 0, pointHoverRadius: 4, tension: 0.25 },
        { label: 'Overall', data: r.all, borderColor: css('--text-secondary'), backgroundColor: css('--text-secondary'), borderWidth: 1.5, borderDash: [5, 4], pointRadius: 0, pointHoverRadius: 4, tension: 0.25 },
      ]
    },
    options: lineOptions('QSOs per hour')
  });
}

async function loadPicker() {
  const ops = (await (await fetch(q('api/operators'))).json()).operators || [];
  const names = ops.map(o => o.name);
  if (op && !names.includes(op)) names.unshift(op);
  el('op-pick').innerHTML = names.map(n => `<option${n === op ? ' selected' : ''}>${esc(n)}</option>`).join('');
}
el('op-pick').onchange = e => { location.href = q('operator.html', { op: e.target.value }); };

onThemeToggle(() => { [runspChart, countryChart, rollingChart].forEach(c => c?.destroy()); runspChart = countryChart = rollingChart = null; if (lastData) render(lastData); });
choiceButtons(el('rate-window'), [[15, '15 min'], [30, '30 min'], [60, '60 min']], rateWindow, v => { rateWindow = v; if (lastData) renderRolling(lastData); });
startClock();
el('dash-link').href = q('./');
el('ops-link').href = q('operators.html');
followUpdates(async () => {
  render(await (await fetch(q('api/operator', { op }))).json());
  await loadPicker();
});
