// Build-time proof that the vendored session-limit patch is actually in effect.
//
// WHY THIS EXISTS. `images/ocp/patches/` carries a one-word fix to OCP's upstream
// rate-limit classifier, vendored because no upstream release contains it yet
// (dtzp555-max/ocp#492). Vendored patches rot in two directions and both are
// silent: the patch can stop applying (upstream edits the same hunk -> the build
// fails, which is the good case), or it can apply and do nothing (upstream adds
// the phrase itself, so the patch becomes a no-op that still reports success and
// keeps us pinned to a fork of a file nobody is watching anymore). This script
// fails the build in the second case, and it asserts the CONTROL as well — the
// patch must not have widened the classifier toward a bare `limit`, which is the
// regression the upstream list's own comments warn about.
//
// DELETE this file and the patch together. See the Dockerfile header.
import { isUpstreamRateLimit } from '/opt/ocp/lib/upstream-errors.mjs';

const WALL = 'hit your session limit';   // apostrophe-free substring of the observed CLI message
const MUST_BE_429 = [
  WALL,
  'Claude usage limit reached',                       // pre-existing phrasing, must be unaffected
  'API error: rate limit exceeded',
];
const MUST_STAY_500 = [
  'FATAL ERROR: heap limit exceeded - JavaScript heap out of memory',  // the control: bare `limit` must not match
  'processed 1429 tokens',
  'pid=429',
  'session not found',                                 // near-miss: the patch must not swallow these
];

const bad = [];
for (const m of MUST_BE_429) if (!isUpstreamRateLimit(m)) bad.push(`should be 429 but is not: ${m}`);
for (const m of MUST_STAY_500) if (isUpstreamRateLimit(m)) bad.push(`must NOT be 429: ${m}`);

if (bad.length) {
  throw new Error(
    `vendored session-limit patch is not doing what it claims (${bad.length} failure(s)):\n  - ` +
    bad.join('\n  - ') +
    '\nIf upstream now carries the phrase, delete the patch and this file; otherwise the patch no longer applies as intended.'
  );
}
console.log(`session-limit classifier patch verified (${MUST_BE_429.length} positive, ${MUST_STAY_500.length} negative rows)`);
