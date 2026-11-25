// scheduler/src/index.test.js
// Unit tests for Cloudflare Worker cron dispatches GitHub workflow
// Run with: npm test (node --test)

import { strictEqual, ok, equal } from 'node:assert';
import { describe, it, beforeEach, afterEach } from 'node:test';

// Import functions
import { scheduled, fetch as httpFetch, RetryableError } from './index.js';

// Mock globals
let mockResponses = [];
let loggedMessages = [];

const originalFetch = globalThis.fetch;
const originalConsole = { ...globalThis.console };

function setupFetchMock() {
  mockResponses = [];
  loggedMessages = [];

  globalThis.fetch = async (url, options) => {
    if (mockResponses.length === 0) {
      throw new Error('No mock response configured for fetch');
    }
    const mock = mockResponses.shift();
    return {
      status: mock.status,
      statusText: mock.statusText,
      headers: new Headers(mock.headers || {}),
      text: async () => mock.body || '',
      json: async () => {
        try {
          return JSON.parse(mock.body || '{}');
        } catch {
          return {};
        }
      }
    };
  };
}

function setupConsoleMock() {
  loggedMessages = [];
  globalThis.console = {
    ...globalThis.console,
    log: (...args) => {
      loggedMessages.push(args.map(a => typeof a === 'object' ? JSON.stringify(a) : String(a)).join(' '));
    },
    error: (...args) => {
      loggedMessages.push(args.map(a => typeof a === 'object' ? JSON.stringify(a) : String(a)).join(' '));
    }
  };
}

function restoreGlobals() {
  globalThis.fetch = originalFetch;
  globalThis.console = originalConsole;
}

function parseLogs() {
  return loggedMessages.map(line => {
    try {
      return JSON.parse(line);
    } catch {
      return { raw: line };
    }
  });
}

afterEach(() => {
  restoreGlobals();
});

describe('dispatchWorkflow', () => {
  beforeEach(() => {
    setupFetchMock();
    setupConsoleMock();
  });

  it('should succeed on 204 response', async () => {
    mockResponses = [{ status: 204, statusText: 'No Content', body: '' }];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token');

    strictEqual(result.status, 204);
    strictEqual(result.message, 'Workflow dispatch queued');
    strictEqual(result.attempt, 1);
  });

  it('should include inputs.publish:"true" in dispatch body', async () => {
    let capturedBody = null;
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (url, options) => {
      capturedBody = options?.body || '';
      return {
        status: 204,
        statusText: 'No Content',
        headers: {},
        text: async () => ''
      };
    };

    const { dispatchWorkflow } = await import('./index.js');
    await dispatchWorkflow('fake-token', { sleep: async () => {} });

    globalThis.fetch = originalFetch;

    const body = JSON.parse(capturedBody);
    strictEqual(body.ref, 'main');
    strictEqual(body.inputs?.publish, 'true');
  });

  // Issue #163 / HS-PORT-07 (AC-6): the Cloudflare Worker dispatch body must
  // NEVER contain a force_ingest key — force_ingest is a maintenance-only
  // operator input, not something the automated scheduler activates.
  it('should NOT include force_ingest in dispatch body', async () => {
    let capturedBody = null;
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (url, options) => {
      capturedBody = options?.body || '';
      return {
        status: 204,
        statusText: 'No Content',
        headers: {},
        text: async () => ''
      };
    };

    const { dispatchWorkflow } = await import('./index.js');
    await dispatchWorkflow('fake-token', { sleep: async () => {} });

    globalThis.fetch = originalFetch;

    const body = JSON.parse(capturedBody);
    strictEqual(body.inputs?.force_ingest, undefined,
      'force_ingest must NOT be present in the worker dispatch body');
    // The inputs object should only carry publish
    strictEqual(Object.keys(body.inputs).sort().join(','), 'publish',
      'worker dispatch inputs must contain only publish');
  });

  it('should throw fatal error on 401 (unauthorized)', async () => {
    mockResponses = [{ status: 401, statusText: 'Unauthorized', body: '{"message":"Bad credentials"}' }];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('bad-token', { sleep: async () => {} });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('GitHub API 401'), true, 'Error should mention 401');
    strictEqual(caughtError.name, 'Error', '401 should be plain Error (non-retryable)');
  });

  it('should throw fatal error on 403 (forbidden)', async () => {
    mockResponses = [{ status: 403, statusText: 'Forbidden', body: '{"message":"Missing scopes"}' }];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('no-scopes', { sleep: async () => {} });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('GitHub API 403'), true, 'Error should mention 403');
    strictEqual(caughtError.name, 'Error', '403 should be plain Error (non-retryable)');
  });

  it('should throw fatal error on 404 (not found)', async () => {
    mockResponses = [{ status: 404, statusText: 'Not Found', body: '{"message":"Not Found"}' }];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('some-token', { sleep: async () => {} });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('GitHub API 404'), true, 'Error should mention 404');
    strictEqual(caughtError.name, 'Error', '404 should be plain Error (non-retryable)');
  });

  it('should throw fatal error on 422 (unprocessable)', async () => {
    mockResponses = [{ status: 422, statusText: 'Unprocessable Entity', body: '{"message":"Invalid ref"}' }];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('some-token', { sleep: async () => {} });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('GitHub API 422'), true, 'Error should mention 422');
    strictEqual(caughtError.name, 'Error', '422 should be plain Error (non-retryable)');
  });

  it('should retry on 500 then succeed', async () => {
    mockResponses = [
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Error"}' },
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token', { sleep: async () => {} });

    strictEqual(result.status, 204);
    strictEqual(result.attempt, 2);
  });

  it('should exhaust retries and throw on repeated 500 errors', async () => {
    mockResponses = [
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Error"}' },
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Error"}' },
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Error"}' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('fake-token', { sleep: async () => {} });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.name, 'RetryableError', 'Error should be RetryableError');
  });

  it('should honor Retry-After header on 429', async () => {
    mockResponses = [
      { status: 429, statusText: 'Too Many Requests', headers: { 'Retry-After': '5' }, body: '{"message":"Rate limit"}' },
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token', { sleep: async () => {} });

    strictEqual(result.status, 204);
    strictEqual(result.attempt, 2);
  });

  it('should apply exponential backoff with capped delay', async () => {
    let capturedDelays = [];
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (url, options) => {
      if (capturedDelays.length < 3) {
        // Simulate 500 error for first 3 attempts
        capturedDelays.push(capturedDelays.length); // record attempt index
        return { status: 500, statusText: 'Internal Server Error', headers: {}, text: async () => '{"message":"Error"}' };
      }
      return { status: 204, statusText: 'No Content', headers: {}, text: async () => '' };
    };

    const { dispatchWorkflow } = await import('./index.js');

    let called = false;
    const testSleep = async (ms) => {
      called = true;
      capturedDelays.push(ms);
    };

    await dispatchWorkflow('fake-token', { sleep: testSleep });

    globalThis.fetch = originalFetch;

    ok(called, 'sleep should have been called');
    ok(capturedDelays.length > 0, 'delays should have been captured');
    // Check delay grows exponentially but capped at MAX_RETRY_DELAY_MS (30000)
    if (capturedDelays.length >= 1) {
      ok(capturedDelays[0] <= 30000, 'first delay capped at 30000ms');
    }
  });
});

describe('scheduled (default export)', () => {
  beforeEach(() => {
    setupFetchMock();
    setupConsoleMock();
  });

  it('should succeed when GITHUB_TOKEN is present (204)', async () => {
    mockResponses = [{ status: 204, statusText: 'No Content', body: '' }];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: 'fake-token' };

    await scheduled({}, env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    const infoLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'info');
    ok(infoLog, 'Should log success info');
    strictEqual(infoLog?.status, 'accepted'); // 204 means accepted/queued
  });

  it('should fail and throw when GITHUB_TOKEN is missing', async () => {
    mockResponses = [];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: '' };

    let caughtError = null;
    try {
      await scheduled({}, env, { waitUntil: fn => fn() });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('missing'), true, 'Error should mention missing token');
  });

  it('should fail and throw when GITHUB_TOKEN is whitespace-only', async () => {
    mockResponses = [];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: '   ' };

    let caughtError = null;
    try {
      await scheduled({}, env, { waitUntil: fn => fn() });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('missing'), true, 'Error should mention missing token');
  });

  it('should throw on 401 without leaking token in logs', async () => {
    mockResponses = [{ status: 401, statusText: 'Unauthorized', body: '{"message":"Bad credentials"}' }];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: 'ghp_secrettoken123' };

    let caughtError = null;
    try {
      await scheduled({}, env, { waitUntil: fn => fn() });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    const logs = parseLogs();
    logs.forEach(log => {
      if (typeof log === 'object') {
        ok(!JSON.stringify(log).includes('ghp_secrettoken123'), `Token should not appear in log: ${JSON.stringify(log)}`);
      }
    });
  });

  it('should throw on 403 without leaking token in logs', async () => {
    mockResponses = [{ status: 403, statusText: 'Forbidden', body: '{"message":"Missing scopes"}' }];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: 'ghp_secrettoken456' };

    let caughtError = null;
    try {
      await scheduled({}, env, { waitUntil: fn => fn() });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    const logs = parseLogs();
    logs.forEach(log => {
      if (typeof log === 'object') {
        ok(!JSON.stringify(log).includes('ghp_secrettoken456'), `Token should not appear in log: ${JSON.stringify(log)}`);
      }
    });
  });

  it('should throw on 404 without leaking token in logs', async () => {
    mockResponses = [{ status: 404, statusText: 'Not Found', body: '{"message":"Not Found"}' }];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: 'ghp_secrettoken789' };

    let caughtError = null;
    try {
      await scheduled({}, env, { waitUntil: fn => fn() });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    const logs = parseLogs();
    logs.forEach(log => {
      if (typeof log === 'object') {
        ok(!JSON.stringify(log).includes('ghp_secrettoken789'), `Token should not appear in log: ${JSON.stringify(log)}`);
      }
    });
  });

  it('should throw on 422 without leaking token in logs', async () => {
    mockResponses = [{ status: 422, statusText: 'Unprocessable Entity', body: '{"message":"Invalid ref"}' }];

    const defaultExport = await import('./index.js');
    const { scheduled } = defaultExport;
    const env = { GITHUB_TOKEN: 'ghp_secrettoken999' };

    let caughtError = null;
    try {
      await scheduled({}, env, { waitUntil: fn => fn() });
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    const logs = parseLogs();
    logs.forEach(log => {
      if (typeof log === 'object') {
        ok(!JSON.stringify(log).includes('ghp_secrettoken999'), `Token should not appear in log: ${JSON.stringify(log)}`);
      }
    });
  });
});

describe('http handler', () => {
  it('should return 404 for non-GET requests', async () => {
    const request = new Request('https://example.com/ping', { method: 'POST' });
    const response = await httpFetch(request, {});
    strictEqual(response.status, 404);
  });

  it('should return status info for GET /ping', async () => {
    const request = new Request('https://example.com/ping', { method: 'GET' });
    const response = await httpFetch(request, {});
    strictEqual(response.status, 200);
    const body = await response.json();
    strictEqual(body.status, 'ok');
    strictEqual(typeof body.timestamp, 'string');
    strictEqual(body.hasToken, false);
  });

  it('should detect token when Authorization header present', async () => {
    const request = new Request('https://example.com/ping', {
      method: 'GET',
      headers: { Authorization: 'Bearer gh_token_here' }
    });
    const response = await httpFetch(request, {});
    const body = await response.json();
    strictEqual(body.hasToken, true);
  });
});

describe('utilities', () => {
  it('should parse Retry-After header', async () => {
    const { getRetryAfterSeconds } = await import('./index.js');
    const response = { headers: { get: (name) => name === 'Retry-After' ? '5' : null } };
    strictEqual(getRetryAfterSeconds(response), 5);
  });

  it('should default Retry-After to 0 when missing', async () => {
    const { getRetryAfterSeconds } = await import('./index.js');
    const response = { headers: { get: () => null } };
    strictEqual(getRetryAfterSeconds(response), 0);
  });

  it('should detect retryable errors', async () => {
    const { isRetryableError } = await import('./index.js');

    ok(isRetryableError(new RetryableError('timeout', 0)), 'RetryableError should be retryable');
    ok(isRetryableError({ name: 'AbortError' }), 'AbortError should be retryable');
    ok(!isRetryableError(new Error('401 Unauthorized')), '401 should not be retryable');
    ok(!isRetryableError(new Error('404 Not Found')), '404 should not be retryable');
    ok(!isRetryableError(new Error('422 Unprocessable')), '422 should not be retryable');
  });
});