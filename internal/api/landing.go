package api

import "net/http"

const landingPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Device Registry API</title>
<style>
body{font-family:system-ui,sans-serif;max-width:42rem;margin:2.5rem auto;padding:0 1rem;line-height:1.55;color:#1a1a1a}
code,pre{background:#f3f3f3;border-radius:4px;font-size:.9em}
code{padding:.1em .3em}
pre{padding:.8rem;overflow-x:auto}
td{padding:.2rem .8rem .2rem 0;vertical-align:top}
</style>
</head>
<body>
<h1>Device Registry API</h1>
<p>A small REST API to manage devices such as cameras and sensors. It uses JWT login with admin and viewer roles, and an ETag / If-Match check so two clients cannot overwrite each other's changes.</p>
<h2>Endpoints</h2>
<table>
<tr><td><code>POST /auth/token</code></td><td>log in, returns a JWT</td></tr>
<tr><td><code>GET /devices</code></td><td>list devices</td></tr>
<tr><td><code>GET /devices/{id}</code></td><td>get one device</td></tr>
<tr><td><code>POST /devices</code></td><td>create (admin)</td></tr>
<tr><td><code>PUT /devices/{id}</code></td><td>update with If-Match (admin)</td></tr>
<tr><td><code>DELETE /devices/{id}</code></td><td>delete (admin)</td></tr>
<tr><td><code>GET /healthz</code></td><td>health check</td></tr>
</table>
<h2>Try it</h2>
<pre>curl -X POST https://device-registry.onrender.com/auth/token \
  -H 'Content-Type: application/json' \
  -d '{"username":"USER","password":"PASSWORD"}'

curl https://device-registry.onrender.com/devices \
  -H 'Authorization: Bearer TOKEN'</pre>
<p>Login details are not public. The Postman collection and full documentation are in the repository.</p>
<p>This runs on a free host, so the first request after a quiet period can take up to a minute.</p>
<p><a href="https://github.com/Ritik-Sharma2311/device-registry">Source code on GitHub</a></p>
</body>
</html>
`

func landing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(landingPage))
}
