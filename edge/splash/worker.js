// Serves a staging splash for any app whose environment is not up.
// `global_fetch_strictly_public` keeps the origin fetch off this Worker's own
// route, which would otherwise trip the subrequest loop limit (error 1019).

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

// 404 is what istio answers for a host nobody claims; 502+ is a cluster that cannot
// serve it. Both mean the same thing here: there is no environment to show.
const missing = (response) => response.status === 404 || response.status >= 502;

function splash(hostname) {
  // staging-openkite.maklab.net -> openkite
  const app = hostname.replace(/^staging-/, "").split(".")[0] || hostname;
  return new Response(SPLASH(app), {
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
