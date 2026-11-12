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

  it('should throw fatal error on 401 (unauthorized)', async () => {
    mockResponses = [{ status: 401, statusText: 'Unauthorized', body: '{"message":"Bad credentials"}' }];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('bad-token');
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('GitHub API 401'), true, 'Error should mention 401');
  });

  it('should throw fatal error on 403 (forbidden)', async () => {
    mockResponses = [{ status: 403, statusText: 'Forbidden', body: '{"message":"Missing scopes"}' }];

    const { dispatchWorkflow } = await import('./index.js');
    let caughtError = null;
    try {
      await dispatchWorkflow('no-scopes');
    } catch (err) {
      caughtError = err;
    }

    ok(caughtError, 'Should have thrown an error');
    strictEqual(caughtError.message.includes('GitHub API 403'), true, 'Error should mention 403');
  });

  it('should retry on 500 then succeed', async () => {
    mockResponses = [
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Error"}' },
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token');

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
      await dispatchWorkflow('fake-token');
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
    const result = await dispatchWorkflow('fake-token');

    strictEqual(result.status, 204);
    strictEqual(result.attempt, 2);
  });
});

describe('scheduled', () => {
  beforeEach(() => {
    setupFetchMock();
    setupConsoleMock();
  });

  it('should succeed when GITHUB_TOKEN is present', async () => {
    mockResponses = [{ status: 204, statusText: 'No Content', body: '' }];

    const env = { GITHUB_TOKEN: 'fake-token' };
    await scheduled(env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    const infoLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'info');
    ok(infoLog, 'Should log success info');
    strictEqual(infoLog?.status, 'dispatched');
  });

  it('should fail when GITHUB_TOKEN is missing', async () => {
    mockResponses = [];

    const env = { GITHUB_TOKEN: '' };
    await scheduled(env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    const errorLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'error');
    ok(errorLog, 'Should log error for missing token');
    ok(errorLog?.error?.includes('missing'), 'Error should mention missing token');
  });

  it('should not log GITHUB_TOKEN in any output', async () => {
    mockResponses = [{ status: 401, statusText: 'Unauthorized', body: '{"message":"Bad credentials"}' }];

    const env = { GITHUB_TOKEN: 'ghp_secrettoken123' };
    await scheduled(env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    logs.forEach(log => {
      if (typeof log === 'object') {
        ok(!JSON.stringify(log).includes('ghp_secrettoken123'), `Token should not appear in log`);
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
  });
});