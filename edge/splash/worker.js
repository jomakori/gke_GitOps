// Serves a staging splash for any app whose environment is not up.
// `global_fetch_strictly_public` keeps the origin fetch off this Worker's own
// route, which would otherwise trip the subrequest loop limit (error 1019).
//
// One Worker answers every app's staging host. The page names the app from the
// hostname, so most apps need nothing here: an app with its own visual identity
// is themed in THEMED, and every other host keeps the generic page untouched.

const SPLASH = (app) => `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>${app} · staging</title>
<style>
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  body {
    margin: 0; min-height: 100vh;
    display: grid; place-items: center; padding: 2rem;
    background: radial-gradient(circle at 50% 0%, #16202e 0%, #0b0f14 60%);
    color: #e6edf3;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  }
  main { max-width: 34rem; text-align: left; }
  .tag {
    display: inline-block; margin-bottom: 1.25rem; padding: .25rem .6rem;
    border: 1px solid #2b3a4b; border-radius: 999px;
    font-size: .75rem; letter-spacing: .08em; text-transform: uppercase;
    color: #7d8ea3;
  }
  h1 { margin: 0 0 .75rem; font-size: clamp(1.6rem, 5vw, 2.4rem); letter-spacing: -.02em; }
  h1 span { color: #58a6ff; }
  p { margin: 0 0 .75rem; line-height: 1.65; color: #a9b7c6; font-size: .95rem; }
  code {
    padding: .12rem .35rem; border-radius: 4px;
    background: #1b2534; color: #d6e2f0;
  }
  footer { margin-top: 2rem; font-size: .75rem; color: #566678; }
</style>
</head>
<body>
<main>
  <div class="tag">staging</div>
  <h1>No <span>${app}</span> environment is running</h1>
  <p>This host answers every staging deployment of <code>${app}</code>, and nothing is deployed here right now.</p>
  <p>Open a pull request and add the <code>staging</code> label: it builds this host, and closing the pull request takes it down again.</p>
  <footer>edge splash · served by Cloudflare Workers</footer>
</main>
</body>
</html>
`;

// Strands' own surfaces, taken from crates/web/assets/main.css: the same oklch
// tokens, the same display/body stacks (system faces, so no webfont request), the
// app's tinted glows and night-sky speckles, its 20px radius and its mark. The app
// ships dark; the splash answers prefers-color-scheme anyway, so a light-mode
// visitor gets a day version of the same palette rather than a dark slab.
const STRANDS_SPLASH = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="color-scheme" content="dark light">
<meta name="theme-color" content="#090d16" media="(prefers-color-scheme: dark)">
<meta name="theme-color" content="#f7f8fb" media="(prefers-color-scheme: light)">
<title>strands · staging</title>
<style>
  :root {
    color-scheme: dark;
    --bg: oklch(0.16 0.02 265);
    --surface: oklch(0.21 0.025 265);
    --fg: oklch(0.97 0.01 265);
    --muted: oklch(0.74 0.015 265);
    --border: color-mix(in oklch, var(--fg) 12%, transparent);
    --accent: oklch(0.86 0.14 95);
    --travel: oklch(0.82 0.12 190);
    --tech: oklch(0.76 0.14 255);
    --ideas: oklch(0.77 0.14 310);
    --link: var(--accent);
    --mark: oklch(0.21 0.025 265);
    --mark-ink: oklch(0.97 0.01 265);
    --font-display: "Space Grotesk", system-ui, -apple-system, "Segoe UI", sans-serif;
    --font-body: "Inter", system-ui, -apple-system, "Segoe UI", sans-serif;
    --radius: 20px;
    --ease: cubic-bezier(0.2, 0, 0, 1);
  }
  @media (prefers-color-scheme: light) {
    :root {
      color-scheme: light;
      --bg: oklch(0.975 0.008 265);
      --surface: oklch(1 0 0);
      --fg: oklch(0.22 0.02 265);
      --muted: oklch(0.47 0.02 265);
      --border: color-mix(in oklch, var(--fg) 16%, transparent);
      --link: oklch(0.5 0.13 75);
    }
  }
  * { box-sizing: border-box; }
  html, body { margin: 0; min-height: 100%; }
  body {
    min-height: 100vh;
    min-height: 100dvh;
    display: flex; align-items: center; justify-content: center;
    padding: calc(env(safe-area-inset-top, 0px) + 20px) 16px calc(env(safe-area-inset-bottom, 0px) + 20px);
    background:
      radial-gradient(900px 600px at 85% -15%, color-mix(in oklch, var(--tech) 14%, transparent), transparent 65%),
      radial-gradient(800px 560px at -5% 105%, color-mix(in oklch, var(--travel) 10%, transparent), transparent 65%),
      var(--bg);
    color: var(--fg);
    font-family: var(--font-body);
    font-size: 14px;
    line-height: 1.5;
    overflow-wrap: break-word;
    -webkit-font-smoothing: antialiased;
  }
  /* the app's night sky: a few tinted speckles, no image request */
  body::before {
    content: ""; position: fixed; inset: 0; pointer-events: none;
    background-image:
      radial-gradient(1px 1px at 12% 22%, color-mix(in oklch, var(--fg) 60%, transparent), transparent 55%),
      radial-gradient(1px 1px at 74% 13%, color-mix(in oklch, var(--fg) 45%, transparent), transparent 55%),
      radial-gradient(1.5px 1.5px at 48% 31%, color-mix(in oklch, var(--tech) 60%, transparent), transparent 60%),
      radial-gradient(1px 1px at 82% 58%, color-mix(in oklch, var(--travel) 50%, transparent), transparent 55%),
      radial-gradient(1.5px 1.5px at 63% 81%, color-mix(in oklch, var(--ideas) 55%, transparent), transparent 60%);
  }
  main {
    position: relative;
    width: 100%; max-width: 30rem; min-width: 0;
    padding: clamp(22px, 6vw, 34px);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: color-mix(in oklch, var(--surface) 72%, transparent);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    box-shadow: 0 24px 60px -36px oklch(0 0 0 / 0.55);
  }
  .brand { display: flex; align-items: center; gap: 10px; min-width: 0; margin-bottom: 20px; }
  .brand-mark { flex: 0 0 auto; width: 32px; height: 32px; border-radius: 9px; display: block; }
  .brand-word {
    font-family: var(--font-display); font-size: 21px; font-weight: 700;
    letter-spacing: -.03em; line-height: 1; color: var(--fg);
  }
  .tag {
    margin-left: auto; flex: 0 0 auto; padding: 3px 9px;
    border: 1px solid var(--border); border-radius: 99px;
    font-size: 11px; letter-spacing: .08em; text-transform: uppercase;
    color: var(--muted);
  }
  h1 {
    margin: 0 0 10px;
    font-family: var(--font-display);
    font-size: clamp(1.3rem, 5.6vw, 1.75rem);
    font-weight: 700; line-height: 1.2; letter-spacing: -.02em;
    text-wrap: balance;
  }
  p { margin: 0; color: var(--muted); font-size: 14px; line-height: 1.6; }
  .hint { margin-top: 8px; font-size: 13px; }
  a {
    color: var(--link);
    text-decoration-line: underline; text-decoration-thickness: 1px;
    text-underline-offset: 3px;
    transition: color .15s var(--ease);
  }
  a:hover { color: var(--fg); }
  :focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; border-radius: 6px; }
  footer { margin-top: 20px; font-size: 11px; color: var(--muted); }
  @media (prefers-reduced-motion: reduce) { a { transition: none; } }
</style>
</head>
<body>
<main>
  <div class="brand">
    <svg class="brand-mark" viewBox="0 0 32 32" width="32" height="32" aria-hidden="true" focusable="false">
      <rect width="32" height="32" rx="9" fill="var(--mark)" />
      <g transform="translate(16 16) scale(.92) translate(-15.4 -14.1)">
        <rect x="3.2" y="11.2" width="22" height="13" rx="6.5" fill="none" stroke="var(--mark-ink)" stroke-width="2.4" />
        <path d="M20 11.2 25 6" fill="none" stroke="var(--mark-ink)" stroke-width="2.4" stroke-linecap="round" />
        <circle cx="10" cy="17.7" r="2.8" fill="var(--accent)" />
        <circle cx="26" cy="5.6" r="2.8" fill="var(--accent)" />
      </g>
    </svg>
    <span class="brand-word">strands</span>
    <span class="tag">staging</span>
  </div>
  <h1>Staging environment is not up.</h1>
  <p>Add the <a href="https://github.com/jomakori/strands/pulls?q=is%3Apr+is%3Aopen+label%3Astaging">staging label</a> to deploy this env.</p>
  <p class="hint">Closing the pull request takes it down again.</p>
  <footer>edge splash · served by Cloudflare Workers</footer>
</main>
</body>
</html>
`;

// Per-app themes, keyed by the app name in the host. An entry only changes the
// host it names; every other staging host still gets the generic SPLASH above.
const THEMED = {
  strands: STRANDS_SPLASH,
};

// 404 is what istio answers for a host nobody claims; 502+ is a cluster that cannot
// serve it. Both mean the same thing here: there is no environment to show.
const missing = (response) => response.status === 404 || response.status >= 502;

function splash(hostname) {
  // staging-openkite.maklab.net -> openkite
  const app = hostname.replace(/^staging-/, "").split(".")[0] || hostname;
  return new Response(THEMED[app] || SPLASH(app), {
    status: 200,
    headers: {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
    },
  });
}

export default {
  async fetch(request) {
    let origin;
    try {
      origin = await fetch(request);
    } catch {
      // Tunnel down or origin refused the connection: same outcome as a 404.
    }
    if (origin && !missing(origin)) return origin;
    return splash(new URL(request.url).hostname);
  },
};
