{{/*
Placeholder page served by the splash VirtualService. Authored from the repo
banner (docs/media/banner.svg in openkite) so the page matches the app's own
artwork; the kite mark is inlined because the response carries no assets.
*/}}
{{- define "splash.page" -}}
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex">
<title>{{ .Values.splash.brand }} staging</title>
<style>
:root { color-scheme: light; }
* { box-sizing: border-box; }
body {
  align-items: center;
  background: linear-gradient(135deg, #f5f7fa 0%, #e8eef7 100%);
  color: #1e2024;
  display: flex;
  font-family: "IBM Plex Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  justify-content: center;
  margin: 0;
  min-height: 100vh;
}
main { max-width: 34rem; padding: 2rem; text-align: center; }
.logo { display: block; height: 81px; margin: 0 auto 1.25rem; width: 72px; }
.brand { font-size: 1.5rem; font-weight: 700; letter-spacing: -0.01em; margin: 0 0 0.5rem; }
.detail { color: #6b7280; font-size: 1rem; line-height: 1.55; margin: 0; }
a { color: #0d9488; font-weight: 600; text-decoration: none; }
a:hover, a:focus { text-decoration: underline; }
</style>
</head>
<body>
<main>
<svg class="logo" viewBox="0 0 32 36" role="img" aria-label="{{ .Values.splash.brand }}">
<path d="M16 3l11.5 10L16 28 4.5 13 16 3z" fill="#0d9488"/>
<path d="M12.2 15 16 13l6-4.8M12.9 20 16 13l6.3 5.2" fill="none" stroke="#ffffff" stroke-linecap="round"
stroke-linejoin="round" stroke-width="1.8"/>
<path d="M16 28v4M11.5 29.2 16 33l4.5-3.8" fill="none" stroke="#0d9488" stroke-linecap="round"
stroke-linejoin="round" stroke-width="1.8"/>
</svg>
<h1 class="brand">{{ .Values.splash.brand }}</h1>
<p class="detail">{{ .Values.splash.message }}<br><a href="{{ .Values.splash.linkUrl }}">{{ .Values.splash.linkText }}</a>.</p>
</main>
</body>
</html>
{{- end -}}
