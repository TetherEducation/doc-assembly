import http from 'node:http';
import { readFile } from 'node:fs/promises';

const port = 8099;
const backend = 'http://localhost:8081';
const documenso = 'http://localhost:3000';
const setup = JSON.parse(await readFile(new URL('./setup-proxy.json', import.meta.url), 'utf8'));

function targetFor(reqUrl) {
  const u = new URL(reqUrl, `http://localhost:${port}`);
  if (u.pathname.startsWith('/documenso/')) {
    const stripped = u.pathname.replace(/^\/documenso/, '') || '/';
    return new URL(stripped + u.search, documenso);
  }
  return new URL(u.pathname + u.search, backend);
}

function rewriteText(text) {
  return text
    .replaceAll('http://localhost:3000', `http://localhost:${port}/documenso`)
    .replaceAll('http://127.0.0.1:3000', `http://localhost:${port}/documenso`)
    .replaceAll('href="/', 'href="/documenso/')
    .replaceAll('src="/', 'src="/documenso/');
}

const server = http.createServer(async (req, res) => {
  try {
    const u = new URL(req.url, `http://localhost:${port}`);
    if (u.pathname === '/' || u.pathname === '/index.html') {
      res.setHeader('content-type', 'text/html; charset=utf-8');
      res.end(`<!doctype html><html lang="es"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width,initial-scale=1"/><title>Visual E2E iframe lifecycle</title><style>body{margin:0;background:#f8fafc;font-family:Inter,system-ui,sans-serif;color:#101828}header{height:64px;display:flex;justify-content:space-between;align-items:center;padding:0 18px;background:white;border-bottom:1px solid #e5e7eb}h1{font-size:18px;margin:0}.meta{font-size:12px;color:#475467}button{border:1px solid #d0d5dd;background:white;border-radius:8px;padding:8px 10px;color:#344054;font-weight:650;font-size:13px;cursor:pointer}button.primary{background:#155eef;color:white;border-color:#155eef}main{height:calc(100vh - 65px)}iframe{width:100%;height:100%;border:0;background:white}code{background:#f2f4f7;border-radius:4px;padding:2px 4px}</style></head><body><header><div><h1>Prueba visual E2E — contrato embebido en iframe</h1><div class="meta">doc=<code>${setup.documentId}</code> · email=<code>${setup.email}</code></div></div><div><button id="load-doc">1. Abrir solicitud</button> <button id="load-sign" class="primary">2. Abrir link de firma</button></div></header><main><iframe id="contract-frame" title="Contrato embebido" src="/public/doc/${setup.documentId}"></iframe></main><script>const doc='/public/doc/${setup.documentId}';const sign='/public/sign/${setup.token}';document.getElementById('load-doc').onclick=()=>document.getElementById('contract-frame').src=doc;document.getElementById('load-sign').onclick=()=>document.getElementById('contract-frame').src=sign;</script></body></html>`);
      return;
    }

    const target = targetFor(req.url);
    const headers = { ...req.headers };
    headers.host = target.host;
    const chunks = [];
    req.on('data', c => chunks.push(c));
    req.on('end', async () => {
      const body = chunks.length ? Buffer.concat(chunks) : undefined;
      const upstream = await fetch(target, { method: req.method, headers, body, redirect: 'manual' });
      const contentType = upstream.headers.get('content-type') || '';
      res.statusCode = upstream.status;
      upstream.headers.forEach((value, key) => {
        const lower = key.toLowerCase();
        if (['content-length','content-encoding','transfer-encoding','content-security-policy','x-frame-options'].includes(lower)) return;
        if (lower === 'location') value = value.replaceAll('http://localhost:3000', `http://localhost:${port}/documenso`).replaceAll('/embed/', '/documenso/embed/');
        res.setHeader(key, value);
      });
      if (contentType.includes('text/html') || contentType.includes('javascript') || contentType.includes('json')) {
        const text = await upstream.text();
        res.end(target.href.startsWith(documenso) ? rewriteText(text) : text);
      } else {
        res.end(Buffer.from(await upstream.arrayBuffer()));
      }
    });
  } catch (err) {
    res.statusCode = 500;
    res.end(String(err?.stack || err));
  }
});
server.listen(port, () => console.log(`visual proxy listening http://localhost:${port}`));
