'use strict';
const $ = id => document.getElementById(id);
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
let cfg = null, canChange = false, logSeq = 0;

async function post(path, body, raw) {
  const r = await fetch(path, {
    method: 'POST', headers: { 'X-Contest-Dashboard': '1', 'Content-Type': raw ? 'text/plain' : 'application/json' },
    body: raw ? body : JSON.stringify(body)
  });
  const text = await r.text();
  if (!r.ok) throw new Error(text.trim());
  return JSON.parse(text);
}

async function loadConfig() {
  cfg = await (await fetch('api/config')).json();
  for (const k of ['my_call', 'my_grid', 'title', 'units', 'map_tiles', 'web_port']) $(k).value = cfg[k] ?? '';
  $('udp_ports').value = (cfg.udp_ports || []).join(', ');
}

$('form').onsubmit = async e => {
  e.preventDefault();
  const c = Object.assign({}, cfg, {
    my_call: $('my_call').value, my_grid: $('my_grid').value, title: $('title').value,
    units: $('units').value, map_tiles: $('map_tiles').value, web_port: Number($('web_port').value),
    udp_ports: $('udp_ports').value.split(/[ ,;]+/).map(Number).filter(n => n > 0)
  });
  try { $('saved').textContent = (await post('api/config', c)).ok; await loadConfig(); status(); }
  catch (err) { $('saved').textContent = err.message; }
};

async function status() {
  const s = await (await fetch('api/status')).json();
  canChange = s.can_change;
  $('readonly').hidden = canChange;
  document.querySelectorAll('input, select, button').forEach(x => x.disabled = !canChange);
  $('version').textContent = 'version ' + s.version + ' · data in ' + s.data_dir;
  const L = s.listener;
  const ports = (L.ports || []).map(p => p.ok
    ? `<span class="status-ok">● port ${p.port} listening</span>`
    : `<span class="status-bad">● port ${p.port}: ${esc(p.error)}</span>`).join('<br>');
  const pk = L.packets || {};
  const total = Object.values(pk).reduce((a, b) => a + b, 0);
  const last = total ? `Last packet ${Math.round((Date.now() - new Date(L.last_at)) / 1000)} s ago from ${esc(L.last_from)}.` : '';
  $('listen').innerHTML = ports + `<p>${total ? `Received ${total} packets: ${Object.entries(pk).map(([k, v]) => `${v} ${esc(k)}`).join(', ')}. ${last}` :
    'Nothing received yet. In N1MM+ open Config, Configure Ports, Mode Control, Winkey etc., then the <b>Broadcast Data</b> tab. Tick <b>Contacts</b>, <b>Radio</b> and <b>Score</b>, and put this computer\'s address and port in each box, for example <code>127.0.0.1:12060</code> if N1MM runs on this same computer.'}</p>`;
  const port = location.port || 80;
  $('urls').innerHTML = 'Other computers on your network can watch at: ' +
    (s.ips || []).map(ip => `<a href="http://${ip}:${port}/">http://${ip}:${port}/</a>`).join(', ');
  $('demo').textContent = s.demo ? 'Stop the demo' : 'Start the demo';
  $('demo').dataset.on = s.demo ? '1' : '';
  $('demo').disabled = !canChange;
}

async function contests() {
  const d = await (await fetch('api/summary')).json();
  const list = d.summary.contests || [];
  $('contests').innerHTML = '<tr><th>Contest</th><th>First contact</th><th>Last contact</th><th class="r">QSOs</th><th></th></tr>' +
    (list.length ? list.map(c => `<tr><td>${esc(c.name)}</td><td>${new Date(c.first).toISOString().slice(0, 16).replace('T', ' ')}</td>
      <td>${new Date(c.last).toISOString().slice(0, 16).replace('T', ' ')}</td><td class="r">${c.qsos}</td>
      <td><a class="btn" href="./?contest=${encodeURIComponent(c.key)}">Show</a>
      <button class="danger" data-key="${esc(c.key)}" ${canChange ? '' : 'disabled'}>Delete</button></td></tr>`).join('')
      : '<tr><td colspan="5" class="muted">No contacts yet.</td></tr>');
  $('contests').querySelectorAll('button[data-key]').forEach(b => b.onclick = async () => {
    if (!confirm('Delete every contact in ' + b.dataset.key + '? This cannot be undone.')) return;
    try { await post('api/contest/delete', { key: b.dataset.key }); } catch (e) { alert(e.message); }
    contests();
  });
}

$('adif').onchange = async e => {
  const f = e.target.files[0];
  if (!f) return;
  $('import-msg').textContent = 'Importing…';
  try {
    const r = await post('api/import?name=' + encodeURIComponent(f.name), await f.text(), true);
    $('import-msg').textContent = r.ok;
  } catch (err) { $('import-msg').textContent = err.message; }
  e.target.value = '';
  contests();
};

$('demo').onclick = async () => {
  try { await post('api/demo', { on: !$('demo').dataset.on }); } catch (e) { alert(e.message); }
  await status(); setTimeout(contests, 500);
};

$('quit').onclick = async () => {
  if (!confirm('Stop the dashboard? Pages will stop updating until you start the program again.')) return;
  try { await post('api/quit', {}); document.body.innerHTML = '<p style="padding:24px">The dashboard program has stopped. Start it again to bring the pages back.</p>'; }
  catch (e) { alert(e.message); }
};

async function pollLog() {
  try {
    const lines = await (await fetch('api/log?since=' + logSeq)).json();
    if (lines.length) {
      logSeq = lines[lines.length - 1].seq;
      const box = $('log');
      const atEnd = box.scrollTop + box.clientHeight >= box.scrollHeight - 5;
      box.textContent += lines.map(l => `${l.time} [${l.tag}] ${l.text}`).join('\n') + '\n';
      if (box.textContent.length > 100000) box.textContent = box.textContent.slice(-80000);
      if (atEnd) box.scrollTop = box.scrollHeight;
    }
  } catch (e) { }
}

(async () => {
  await loadConfig();
  await status();
  contests();
  pollLog();
  setInterval(() => { status(); pollLog(); }, 3000);
  setInterval(contests, 10000);
})();
