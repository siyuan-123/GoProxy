package webui

const loginHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ProxyGo - 登录</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f172a;color:#e2e8f0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#1e293b;border:1px solid #334155;border-radius:12px;padding:40px;width:360px;box-shadow:0 20px 60px rgba(0,0,0,0.5)}
h1{font-size:24px;font-weight:700;margin-bottom:8px;color:#f1f5f9}
.sub{color:#94a3b8;font-size:14px;margin-bottom:32px}
label{display:block;font-size:13px;color:#94a3b8;margin-bottom:6px}
input[type=password]{width:100%;padding:10px 14px;background:#0f172a;border:1px solid #334155;border-radius:8px;color:#f1f5f9;font-size:15px;outline:none;transition:border 0.2s}
input[type=password]:focus{border-color:#6366f1}
button{width:100%;margin-top:20px;padding:11px;background:#6366f1;color:#fff;border:none;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer;transition:background 0.2s}
button:hover{background:#4f46e5}
.logo{font-size:32px;margin-bottom:16px}
</style>
</head>
<body>
<div class="card">
  <div class="logo">⚡</div>
  <h1>ProxyGo</h1>
  <p class="sub">代理池管理系统</p>
  <form method="POST" action="/login">
    <label>管理密码</label>
    <input type="password" name="password" placeholder="请输入密码" autofocus>
    <button type="submit">登录</button>
  </form>
</div>
</body>
</html>`

const loginHTMLWithError = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ProxyGo - 登录</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{background:#0f172a;color:#e2e8f0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{background:#1e293b;border:1px solid #334155;border-radius:12px;padding:40px;width:360px;box-shadow:0 20px 60px rgba(0,0,0,0.5)}
h1{font-size:24px;font-weight:700;margin-bottom:8px;color:#f1f5f9}
.sub{color:#94a3b8;font-size:14px;margin-bottom:32px}
label{display:block;font-size:13px;color:#94a3b8;margin-bottom:6px}
input[type=password]{width:100%;padding:10px 14px;background:#0f172a;border:1px solid #334155;border-radius:8px;color:#f1f5f9;font-size:15px;outline:none;transition:border 0.2s}
input[type=password]:focus{border-color:#6366f1}
button{width:100%;margin-top:20px;padding:11px;background:#6366f1;color:#fff;border:none;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer;transition:background 0.2s}
button:hover{background:#4f46e5}
.logo{font-size:32px;margin-bottom:16px}
.error{background:#450a0a;border:1px solid #7f1d1d;color:#fca5a5;padding:10px 14px;border-radius:8px;font-size:13px;margin-bottom:16px}
</style>
</head>
<body>
<div class="card">
  <div class="logo">⚡</div>
  <h1>ProxyGo</h1>
  <p class="sub">代理池管理系统</p>
  <div class="error">密码错误，请重试</div>
  <form method="POST" action="/login">
    <label>管理密码</label>
    <input type="password" name="password" placeholder="请输入密码" autofocus>
    <button type="submit">登录</button>
  </form>
</div>
</body>
</html>`

// dashboardHTML 已移至 dashboard.go
