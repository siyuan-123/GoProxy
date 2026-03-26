package webui

const dashboardHTML = dashboardHTMLHead + dashboardHTMLBody + dashboardHTMLJS

const dashboardHTMLHead = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ProxyGo - 管理面板</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f172a;color:#e2e8f0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif}
.nav{background:#1e293b;border-bottom:1px solid #334155;padding:0 24px;display:flex;align-items:center;justify-content:space-between;height:56px}
.nav-brand{font-size:18px;font-weight:700;color:#f1f5f9;display:flex;align-items:center;gap:8px}
.nav-brand span{color:#6366f1}
.nav-right{display:flex;align-items:center;gap:16px}
.nav-link{color:#94a3b8;text-decoration:none;font-size:14px;cursor:pointer;background:none;border:none;padding:0}
.nav-link:hover{color:#f1f5f9}
.container{max-width:1200px;margin:0 auto;padding:24px}
.stats{display:grid;grid-template-columns:repeat(3,1fr);gap:16px;margin-bottom:24px}
.stat-card{background:#1e293b;border:1px solid #334155;border-radius:10px;padding:20px}
.stat-label{font-size:13px;color:#94a3b8;margin-bottom:8px}
.stat-value{font-size:32px;font-weight:700;color:#f1f5f9}
.stat-value.green{color:#4ade80}
.stat-value.blue{color:#60a5fa}
.stat-value.purple{color:#a78bfa}
.section{background:#1e293b;border:1px solid #334155;border-radius:10px;margin-bottom:24px}
.section-header{padding:16px 20px;border-bottom:1px solid #334155;display:flex;align-items:center;justify-content:space-between}
.section-title{font-size:15px;font-weight:600;color:#f1f5f9}
.tabs{display:flex;gap:4px}
.tab{padding:6px 14px;border-radius:6px;font-size:13px;cursor:pointer;border:none;background:transparent;color:#94a3b8;transition:all 0.2s}
.tab.active{background:#6366f1;color:#fff}
.tab:hover:not(.active){background:#334155;color:#f1f5f9}
.btn{padding:7px 14px;border-radius:6px;font-size:13px;font-weight:500;cursor:pointer;border:none;transition:all 0.2s}
.btn-primary{background:#6366f1;color:#fff}
.btn-primary:hover{background:#4f46e5}
.btn-danger{background:#dc2626;color:#fff;padding:4px 10px;font-size:12px}
.btn-danger:hover{background:#b91c1c}
.btn-sm{padding:4px 10px;font-size:12px;background:#334155;color:#94a3b8;border:none;border-radius:6px;cursor:pointer}
.btn-sm:hover{color:#f1f5f9}
table{width:100%;border-collapse:collapse}
th{padding:10px 16px;text-align:left;font-size:12px;color:#64748b;font-weight:500;border-bottom:1px solid #1e293b;background:#0f172a}
td{padding:10px 16px;font-size:13px;border-bottom:1px solid #1e293b;color:#cbd5e1}
tr:last-child td{border-bottom:none}
tr:hover td{background:#1a2744}
.badge{display:inline-block;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:500}
.badge-http{background:#1e3a5f;color:#60a5fa}
.badge-socks5{background:#2d1b4e;color:#a78bfa}
.log-box{padding:16px 20px;font-family:monospace;font-size:12px;color:#94a3b8;max-height:400px;overflow-y:auto;line-height:1.6}
.log-line{padding:2px 0}
.log-line.error{color:#f87171}
.log-line.success{color:#4ade80}
.empty{padding:40px;text-align:center;color:#475569;font-size:14px}
.refresh-info{font-size:12px;color:#475569}
.proxy-port{font-size:13px;color:#64748b}
.pagination{display:flex;align-items:center;gap:6px;padding:12px 20px;border-top:1px solid #1e293b;flex-wrap:wrap}
.page-btn{padding:4px 10px;border-radius:6px;font-size:13px;cursor:pointer;border:1px solid #334155;background:#0f172a;color:#94a3b8;transition:all 0.2s}
.page-btn:hover:not(:disabled){border-color:#6366f1;color:#f1f5f9}
.page-btn:disabled{opacity:0.4;cursor:default}
.page-btn.active{background:#6366f1;border-color:#6366f1;color:#fff}
.page-info{font-size:12px;color:#475569;margin-left:8px}
.modal-overlay{display:none;position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:100;align-items:center;justify-content:center}
.modal-overlay.show{display:flex}
.modal{background:#1e293b;border:1px solid #334155;border-radius:12px;padding:28px;width:440px;box-shadow:0 20px 60px rgba(0,0,0,0.5)}
.modal-title{font-size:16px;font-weight:700;color:#f1f5f9;margin-bottom:20px}
.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:14px;margin-bottom:20px}
.form-group label{display:block;font-size:12px;color:#94a3b8;margin-bottom:6px}
.form-group input{width:100%;padding:8px 12px;background:#0f172a;border:1px solid #334155;border-radius:6px;color:#f1f5f9;font-size:14px;outline:none}
.form-group input:focus{border-color:#6366f1}
.modal-actions{display:flex;gap:8px;align-items:center;justify-content:flex-end}
.save-tip{font-size:12px;color:#4ade80;margin-right:auto}
</style>
</head>`

const dashboardHTMLBody = `
<body>
<nav class="nav">
  <div class="nav-brand">⚡ <span>Proxy</span>Go</div>
  <div class="nav-right">
    <span id="proxy-port" class="proxy-port"></span>
    <button class="nav-link" onclick="openSettings()">系统设置</button>
    <a href="/logout" class="nav-link">退出登录</a>
  </div>
</nav>

<div class="modal-overlay" id="settings-modal">
  <div class="modal">
    <div class="modal-title">系统设置</div>
    <div class="form-grid">
      <div class="form-group"><label>抓取间隔（分钟）</label><input type="number" id="cfg-fetch" min="1"></div>
      <div class="form-group"><label>健康检查间隔（分钟）</label><input type="number" id="cfg-check" min="1"></div>
      <div class="form-group"><label>验证并发数</label><input type="number" id="cfg-concurrency" min="1"></div>
      <div class="form-group"><label>验证超时（秒）</label><input type="number" id="cfg-timeout" min="1"></div>
    </div>
    <div class="modal-actions">
      <span class="save-tip" id="save-tip" style="display:none">已保存并生效</span>
      <button class="btn-sm" onclick="closeSettings()">取消</button>
      <button class="btn btn-primary" onclick="saveConfig()">保存</button>
    </div>
  </div>
</div>

<div class="container">
  <div class="stats">
    <div class="stat-card"><div class="stat-label">全部代理</div><div class="stat-value green" id="stat-total">-</div></div>
    <div class="stat-card"><div class="stat-label">HTTP 代理</div><div class="stat-value blue" id="stat-http">-</div></div>
    <div class="stat-card"><div class="stat-label">SOCKS5 代理</div><div class="stat-value purple" id="stat-socks5">-</div></div>
  </div>
  <div class="section">
    <div class="section-header">
      <div style="display:flex;align-items:center;gap:12px">
        <span class="section-title">代理列表</span>
        <div class="tabs">
          <button class="tab active" onclick="switchTab('')" id="tab-all">全部</button>
          <button class="tab" onclick="switchTab('http')" id="tab-http">HTTP</button>
          <button class="tab" onclick="switchTab('socks5')" id="tab-socks5">SOCKS5</button>
        </div>
      </div>
      <div style="display:flex;gap:8px;align-items:center">
        <span class="refresh-info" id="proxy-count"></span>
        <button class="btn btn-primary" id="fetch-btn" onclick="triggerFetch()">立即抓取</button>
      </div>
    </div>
    <div id="proxy-table-wrap"><div class="empty">加载中...</div></div>
    <div class="pagination" id="pagination"></div>
  </div>
  <div class="section">
    <div class="section-header">
      <span class="section-title">运行日志</span>
      <button class="btn-sm" onclick="loadLogs()">刷新</button>
    </div>
    <div class="log-box" id="log-box">加载中...</div>
  </div>
</div>`

const dashboardHTMLJS = `
<script>
const PAGE_SIZE = 50;
var currentProtocol = '';
var allProxies = [];
var currentPage = 1;

async function api(path, opts) {
  var r = await fetch(path, opts);
  if (r.status === 401) { location.href = '/login'; return null; }
  return r.json();
}

async function loadStats() {
  var d = await api('/api/stats');
  if (!d) return;
  document.getElementById('stat-total').textContent = d.total;
  document.getElementById('stat-http').textContent = d.http;
  document.getElementById('stat-socks5').textContent = d.socks5;
  document.getElementById('proxy-port').textContent = '代理端口: ' + d.port;
}

function switchTab(protocol) {
  currentProtocol = protocol;
  currentPage = 1;
  ['all','http','socks5'].forEach(function(t) {
    document.getElementById('tab-'+t).className = 'tab' + (t === (protocol||'all') ? ' active' : '');
  });
  loadProxies();
}

async function loadProxies() {
  var d = await api('/api/proxies?protocol=' + currentProtocol);
  if (!d) return;
  allProxies = Array.isArray(d) ? d : [];
  document.getElementById('proxy-count').textContent = allProxies.length + ' 个';
  renderPage();
}

function renderPage() {
  var total = allProxies.length;
  var totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  if (currentPage > totalPages) currentPage = totalPages;
  var start = (currentPage - 1) * PAGE_SIZE;
  var page = allProxies.slice(start, start + PAGE_SIZE);
  if (total === 0) {
    document.getElementById('proxy-table-wrap').innerHTML = '<div class="empty">暂无代理</div>';
    document.getElementById('pagination').innerHTML = '';
    return;
  }
  var html = '<table><thead><tr><th>地址</th><th>协议</th><th>添加时间</th><th>操作</th></tr></thead><tbody>';
  page.forEach(function(p) {
    var badge = p.Protocol === 'http' ? 'badge-http' : 'badge-socks5';
    var t = new Date(p.CreatedAt).toLocaleString('zh-CN');
    html += '<tr><td style="font-family:monospace">' + p.Address + '</td>' +
      '<td><span class="badge ' + badge + '">' + p.Protocol + '</span></td>' +
      '<td>' + t + '</td>' +
      '<td><button class="btn btn-danger" onclick="deleteProxy(\'' + p.Address + '\')">删除</button></td></tr>';
  });
  html += '</tbody></table>';
  document.getElementById('proxy-table-wrap').innerHTML = html;
  var pag = '<button class="page-btn" onclick="goPage(' + (currentPage-1) + ')"' + (currentPage===1?' disabled':'') + '>上一页</button>';
  var sp = Math.max(1, currentPage-3), ep = Math.min(totalPages, sp+6);
  if (ep-sp < 6) sp = Math.max(1, ep-6);
  for (var i = sp; i <= ep; i++) {
    pag += '<button class="page-btn' + (i===currentPage?' active':'') + '" onclick="goPage(' + i + ')">' + i + '</button>';
  }
  pag += '<button class="page-btn" onclick="goPage(' + (currentPage+1) + ')"' + (currentPage===totalPages?' disabled':'') + '>下一页</button>';
  pag += '<span class="page-info">' + currentPage + ' / ' + totalPages + ' 页，共 ' + total + ' 条</span>';
  document.getElementById('pagination').innerHTML = pag;
}

function goPage(p) {
  var totalPages = Math.ceil(allProxies.length / PAGE_SIZE) || 1;
  if (p < 1 || p > totalPages) return;
  currentPage = p;
  renderPage();
}

async function deleteProxy(address) {
  if (!confirm('确认删除 ' + address + ' ?')) return;
  await api('/api/proxy/delete', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({address:address})});
  loadProxies();
  loadStats();
}

async function triggerFetch() {
  var btn = document.getElementById('fetch-btn');
  btn.disabled = true;
  btn.textContent = '抓取中...';
  await api('/api/fetch', {method:'POST'});
  setTimeout(function() {
    btn.disabled = false;
    btn.textContent = '立即抓取';
    loadStats();
    loadProxies();
  }, 3000);
}

async function loadLogs() {
  var d = await api('/api/logs');
  if (!d || !d.lines) return;
  var box = document.getElementById('log-box');
  if (d.lines.length === 0) { box.innerHTML = '<div class="empty">暂无日志</div>'; return; }
  box.innerHTML = d.lines.map(function(l) {
    var cls = 'log-line';
    if (l.indexOf('error') >= 0 || l.indexOf('Error') >= 0 || l.indexOf('failed') >= 0) cls += ' error';
    else if (l.indexOf('valid=') >= 0 || l.indexOf('pool size') >= 0) cls += ' success';
    return '<div class="' + cls + '">' + l + '</div>';
  }).join('');
  box.scrollTop = box.scrollHeight;
}

function openSettings() {
  api('/api/config').then(function(d) {
    if (!d) return;
    document.getElementById('cfg-fetch').value = d.fetch_interval;
    document.getElementById('cfg-check').value = d.check_interval;
    document.getElementById('cfg-concurrency').value = d.validate_concurrency;
    document.getElementById('cfg-timeout').value = d.validate_timeout;
    document.getElementById('settings-modal').style.display = 'flex';
  });
}

function closeSettings() {
  document.getElementById('settings-modal').style.display = 'none';
}

async function saveConfig() {
  var body = {
    fetch_interval: parseInt(document.getElementById('cfg-fetch').value),
    check_interval: parseInt(document.getElementById('cfg-check').value),
    validate_concurrency: parseInt(document.getElementById('cfg-concurrency').value),
    validate_timeout: parseInt(document.getElementById('cfg-timeout').value)
  };
  if (Object.values(body).some(function(v){return isNaN(v)||v<=0;})) {
    alert('所有值必须为正整数');
    return;
  }
  var d = await api('/api/config', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  if (d && d.status === 'saved') {
    var tip = document.getElementById('save-tip');
    tip.style.display = 'inline';
    setTimeout(function(){tip.style.display='none';}, 2000);
  }
}

loadStats();
loadProxies();
loadLogs();
setInterval(loadStats, 10000);
setInterval(loadProxies, 15000);
setInterval(loadLogs, 10000);
</script>
</body>
</html>`
