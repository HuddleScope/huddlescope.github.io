// scheduler/src/index.js
// HS-019: Cloudflare Worker cron dispatches GitHub workflow every 30 minutes
// Cron: 7,37 * * * * UTC triggers workflow_dispatch on the default branch

const GITHUB_API_BASE = 'https://api.github.com';
const REPO_OWNER = 'HuddleScope';
const REPO_NAME = 'huddlescope.github.io';
const WORKFLOW_NAME = 'refresh-news.yml';
const DEFAULT_BRANCH = 'main';

/**
 * Scheduled entry point — invoked by Cloudflare cron triggers
 * @param {Object} env - Cloudflare environment bindings
 * @param {Object} ctx - Cloudflare execution context
 * @param {string} env.GITHUB_TOKEN - Fine-grained GitHub token with Actions:write
 */
export async function scheduled(env, ctx) {
  const startTime = Date.now();
  const logCtx = { event: 'scheduled_dispatch', repo: `${REPO_OWNER}/${REPO_NAME}`, workflow: WORKFLOW_NAME };

  try {
    // Validate secret exists
    if (!env.GITHUB_TOKEN || typeof env.GITHUB_TOKEN !== 'string' || env.GITHUB_TOKEN.trim() === '') {
      const err = new Error('GITHUB_TOKEN secret is missing or empty');
      err.name = 'SecretError';
      console.error(JSON.stringify({ ...logCtx, level: 'error', error: err.message }));
      throw err;
    }

    // Dispatch the workflow
    const dispatchResult = await dispatchWorkflow(env.GITHUB_TOKEN);
    const elapsed = Date.now() - startTime;

    console.log(JSON.stringify({
      ...logCtx,
      level: 'info',
      status: 'dispatched',
      durationMs: elapsed,
      dispatchResult
    }));

    // Note: 204 Accepted means queued, not completed. GitHub API returns 204
    // immediately; the actual workflow run executes asynchronously.
    // See README for 204 semantics and observability guidance.

  } catch (err) {
    const elapsed = Date.now() - startTime;
    const isNetworkOrRetryable = isRetryableError(err);

    console.error(JSON.stringify({
      ...logCtx,
      level: isNetworkOrRetryable ? 'warn' : 'error',
      error: err.message,
      stack: err.stack?.split('\n')[0] || '',
      durationMs: elapsed,
      retryable: isNetworkOrRetryable
    }));

    if (isNetworkOrRetryable) {
      // Re-throw to signal failure to Cloudflare (so triggers/logs show failure)
      throw err;
    }
    // Non-retryable errors (e.g., auth) do not need to block other triggers
  }
}

/**
 * HTTP entry point — optional health probe (curl -H "Authorization: Bearer <token>")
 * @param {Request} request
 * @param {Object} env
 */
export async function fetch(request, env) {
  // Only allow GET at /ping
  if (request.method !== 'GET' || !request.url.endsWith('/ping')) {
    return new Response('Not Found', { status: 404 });
  }

  // Check for valid token (soft validation: token presence, not scope)
  const auth = request.headers.get('Authorization') || '';
  const hasToken = auth.startsWith('Bearer ') && auth.length > 'Bearer '.length;

  return new Response(JSON.stringify({
    status: 'ok',
    timestamp: new Date().toISOString(),
    hasToken
  }), {
    headers: { 'content-type': 'application/json' }
  });
}

/**
 * Dispatch the refresh-news workflow on the default branch
 * Implements explicit status checks, timeout, bounded retries, and structured logging
 * @param {string} token - GitHub token with_actions:write
 * @returns {Promise<Object>} Dispatch result metadata
 */
async function dispatchWorkflow(token) {
  const url = `${GITHUB_API_BASE}/repos/${REPO_OWNER}/${REPO_NAME}/actions/workflows/${encodeURIComponent(WORKFLOW_NAME)}/dispatches`;
  const body = JSON.stringify({ ref: DEFAULT_BRANCH });

  // Configuration
  const REQUEST_TIMEOUT_MS = 10_000; // 10 seconds as per AC3
  const MAX_RETRIES = 3;
  const BASE_RETRY_DELAY_MS = 2_000;

  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    const abortController = new AbortController();
    const timeoutId = setTimeout(() => abortController.abort(), REQUEST_TIMEOUT_MS);

    try {
      const response = await globalThis.fetch(url, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Accept': 'application/vnd.github+json',
          'User-Agent': 'huddlescope-cf-worker/0.1',
          'Content-Type': 'application/json'
        },
        body,
        signal: abortController.signal
      });

      clearTimeout(timeoutId);

      // Explicit HTTP status checks
      if (response.status === 204) {
        // 204 Accepted — queued successfully
        return { status: 204, message: 'Workflow dispatch queued', attempt };
      } else if (response.status === 401 || response.status === 403) {
        // Fatal: token expired, revoked, or wrong scopes
        const bodyText = await safeReadResponse(response);
        throw new Error(`GitHub API ${response.status} (auth/scopes issue): ${bodyText}`);
      } else if (response.status === 404) {
        // Fatal: workflow or repo not found
        const bodyText = await safeReadResponse(response);
        throw new Error(`GitHub API ${response.status}: ${bodyText}`);
      } else if (response.status === 422) {
        // Fatal: unprocessable (e.g., invalid ref, workflow disabled)
        const bodyText = await safeReadResponse(response);
        throw new Error(`GitHub API ${response.status} (unprocessable): ${bodyText}`);
      } else if (response.status === 429) {
        // Retryable: rate limited
        const retryAfter = getRetryAfterSeconds(response);
        throw new RetryableError(`GitHub API ${response.status} (rate-limited, retryAfter=${retryAfter}s)`, retryAfter);
      } else if (response.status >= 500) {
        // Retryable: server error
        const bodyText = await safeReadResponse(response);
        throw new RetryableError(`GitHub API ${response.status} (server error): ${bodyText}`);
      } else if (response.status >= 400) {
        // Other 4xx: treat as fatal
        const bodyText = await safeReadResponse(response);
        throw new Error(`GitHub API ${response.status}: ${bodyText}`);
      }

      // Unexpected status
      throw new Error(`Unexpected HTTP status ${response.status}`);

    } catch (err) {
      clearTimeout(timeoutId);

      if (err.name === 'AbortError') {
        if (attempt < MAX_RETRIES) {
          console.warn(JSON.stringify({ event: 'timeout', attempt, max: MAX_RETRIES }));
          await sleepWithBackoff(attempt, BASE_RETRY_DELAY_MS);
          continue;
        }
        throw new RetryableError('Request timeout', 0);
      }

      if (err instanceof RetryableError && attempt < MAX_RETRIES) {
        console.warn(JSON.stringify({ event: 'retryable_error', attempt, max: MAX_RETRIES, message: err.message, delayMs: err.retryDelayMs }));
        await sleep(err.retryDelayMs);
        continue;
      }

      if (err instanceof RetryableError) {
        console.error(JSON.stringify({ event: 'retry_exhausted', attempt, max: MAX_RETRIES, message: err.message }));
        throw err;
      }

      throw err;
    }
  }

  // Should never reach here, but ensure a return
  throw new Error('Retry loop terminated unexpectedly');
}

/**
 * Utility: retryable error wrapper
 * @param {string} message
 * @param {number} retryDelayMs
 */
class RetryableError extends Error {
  constructor(message, retryDelayMs) {
    super(message);
    this.name = 'RetryableError';
    this.retryDelayMs = retryDelayMs || 0;
  }
}

/**
 * Utility: determine if error is retryable
 * @param {Error} err
 * @returns {boolean}
 */
function isRetryableError(err) {
  if (err.name === 'AbortError') return true;
  if (err instanceof RetryableError) return true;
  // Network errors often have 'TypeError: Failed to fetch' or 'FetchError'
  if (err.message?.includes('Failed to fetch')) return true;
  if (err.cause?.message?.includes('ETIMEDOUT')) return true;
  if (err.cause?.message?.includes('ECONNRESET')) return true;
  return false;
}

/**
 * Utility: read response body safely (no throw on empty body)
 * @param {Response} response
 * @returns {Promise<string>}
 */
async function safeReadResponse(response) {
  try {
    const text = await response.text();
    return text || '<empty body>';
  } catch {
    return '<body read error>';
  }
}

/**
 * Utility: parse Retry-After header (seconds)
 * @param {Response} response
 * @returns {number}
 */
function getRetryAfterSeconds(response) {
  const header = response.headers.get('Retry-After');
  if (!header) return 0;
  const seconds = parseInt(header, 10);
  return isNaN(seconds) ? 0 : seconds;
}

/**
 * Utility: sleep with exponential backoff
 * @param {number} attempt
 * @param {number} baseMs
 * @returns {Promise<void>}
 */
async function sleepWithBackoff(attempt, baseMs) {
  const delay = baseMs * Math.pow(2, attempt - 1); // exponential: 2s, 4s, 8s...
  await sleep(delay);
}

/**
 * Utility: sleep (used for Retry-After or capped backoff)
 * @param {number} ms
 */
async function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

// Export functions for testing
export { dispatchWorkflow, RetryableError, getRetryAfterSeconds, isRetryableError, sleep, sleepWithBackoff, safeReadResponse };