// scheduler/src/index.test.js
// Unit tests for Cloudflare Worker cron dispatches GitHub workflow
// Run with: npm test (node --test)

import { strictEqual, throws, doesNotThrow, ok } from 'node:assert';
import { describe, it, beforeEach, afterEach } from 'node:test';

// Mock fetch and globals
const originalFetch = globalThis.fetch;
const originalConsoleLog = global.console.log;
const originalConsoleError = global.console.error;
const originalDateNow = Date.now;

let mockResponses = [];
let mockResponseIndex = 0;
let loggedMessages = [];

globalThis.fetch = async (url, options) => {
  if (mockResponseIndex >= mockResponses.length) {
    throw new Error('No mock response configured for fetch');
  }
  const mock = mockResponses[mockResponseIndex++];
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

globalThis.console = {
  ...globalThis.console,
  log: (...args) => {
    loggedMessages.push(args.map(a => typeof a === 'object' ? JSON.stringify(a) : String(a)).join(' '));
  },
  error: (...args) => {
    loggedMessages.push(args.map(a => typeof a === 'object' ? JSON.stringify(a) : String(a)).join(' '));
  }
};

Date.now = () => originalDateNow();

// Import after setting up mocks
import { scheduled, fetch as httpFetch } from './index.js';

// Helper to parse logged JSON
function parseLogs() {
  return loggedMessages
    .map(line => {
      try {
        return JSON.parse(line);
      } catch {
        return { raw: line };
      }
    });
}

function resetMocks() {
  mockResponseIndex = 0;
  loggedMessages = [];
}

// Restore after each test
afterEach(() => {
  globalThis.fetch = originalFetch;
  global.console = { log: originalConsoleLog, error: originalConsoleError };
  Date.now = originalDateNow;
});

describe('dispatchWorkflow', () => {
  beforeEach(() => {
    resetMocks();
  });

  it('should succeed on 204 response', async () => {
    mockResponses = [
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token');

    strictEqual(result.status, 204);
    strictEqual(result.message, 'Workflow dispatch queued');
    strictEqual(result.attempt, 1);

    const logs = parseLogs();
    const infoLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'info');
    ok(infoLog, 'Should have info log for success');
    ok(!infoLog?.error, 'Info log should not contain error');
  });

  it('should throw fatal error on 401 (unauthorized)', async () => {
    mockResponses = [
      { status: 401, statusText: 'Unauthorized', body: '{"message":"Bad credentials"}' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    await throws(async () => {
      await dispatchWorkflow('bad-token');
    }, { message: /GitHub API 401/ });

    const logs = parseLogs();
    const errorLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'error');
    ok(errorLog, 'Should have error log for 401');
  });

  it('should throw fatal error on 403 (forbidden)', async () => {
    mockResponses = [
      { status: 403, statusText: 'Forbidden', body: '{"message":"Missing required scopes"}' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    await throws(async () => {
      await dispatchWorkflow('token-no-scopes');
    }, { message: /GitHub API 403/ });

    const logs = parseLogs();
    const errorLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'error');
    ok(errorLog, 'Should have error log for 403');
  });

  it('should retry on 500 then succeed', async () => {
    mockResponses = [
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Internal Server Error"}' },
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token');

    strictEqual(result.status, 204);
    strictEqual(result.attempt, 2);

    const logs = parseLogs();
    const retryLog = logs.find(l => l.event === 'retryable_error');
    ok(retryLog, 'Should log retry attempt');
  });

  it('should exhaust retries and throw on持续 500 errors', async () => {
    mockResponses = [
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Internal Server Error"}' },
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Internal Server Error"}' },
      { status: 500, statusText: 'Internal Server Error', body: '{"message":"Internal Server Error"}' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    await throws(async () => {
      await dispatchWorkflow('fake-token');
    }, { name: 'RetryableError' });

    const logs = parseLogs();
    const exhaustionLog = logs.find(l => l.event === 'retry_exhausted');
    ok(exhaustionLog, 'Should log retry exhaustion');
    strictEqual(exhaustionLog?.attempt, 3);
    strictEqual(exhaustionLog?.max, 3);
  });

  it('should honor Retry-After header on 429', async () => {
    mockResponses = [
      { status: 429, statusText: 'Too Many Requests', headers: { 'Retry-After': '5' }, body: '{"message":"Rate limit exceeded"}' },
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    const result = await dispatchWorkflow('fake-token');

    strictEqual(result.status, 204);
    strictEqual(result.attempt, 2);

    const logs = parseLogs();
    const retryLog = logs.find(l => l.event === 'retryable_error');
    ok(retryLog?.delayMs >= 5000, `Should delay at least Retry-After seconds, got ${retryLog?.delayMs}ms`);
  });

  it('should throw on timeout (408 after 3 retries)', async () => {
    // Configure fetch to throw AbortError
    globalThis.fetch = async (url, options) => {
      if (mockResponseIndex >= mockResponses.length) {
        const err = new Error('Request Timeout');
        err.name = 'AbortError';
        throw err;
      }
      const mock = mockResponses[mockResponseIndex++];
      return {
        status: mock.status,
        statusText: mock.statusText,
        headers: new Headers(mock.headers || {}),
        text: async () => mock.body || ''
      };
    };

    // Reset for the timeout test
    mockResponses = [];
    mockResponseIndex = 0;

    const { dispatchWorkflow } = await import('./index.js');
    await throws(async () => {
      await dispatchWorkflow('fake-token');
    }, { name: 'RetryableError' });
  });

  it('should log dispatchResult on success', async () => {
    mockResponses = [
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { dispatchWorkflow } = await import('./index.js');
    await dispatchWorkflow('fake-token');

    const logs = parseLogs();
    const infoLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'info');
    ok(infoLog?.dispatchResult?.status === 204, 'Should log dispatchResult with status 204');
  });
});

describe('scheduled', () => {
  beforeEach(() => {
    resetMocks();
    Date.now = () => 1731000000000; // Stable timestamp for tests
  });

  it('should succeed when GITHUB_TOKEN is present', async () => {
    mockResponses = [
      { status: 204, statusText: 'No Content', body: '' }
    ];

    const { scheduled } = await import('./index.js');
    const env = { GITHUB_TOKEN: 'fake-token' };
    await scheduled(env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    const infoLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'info');
    ok(infoLog, 'Should log success info');
  });

  it('should fail when GITHUB_TOKEN is missing', async () => {
    mockResponses = [];

    const { scheduled } = await import('./index.js');
    const env = { GITHUB_TOKEN: '' }; // Empty token
    await scheduled(env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    const errorLog = logs.find(l => l.event === 'scheduled_dispatch' && l.level === 'error');
    ok(errorLog, 'Should log error for missing token');
    ok(errorLog?.error?.includes('missing'), 'Error should mention missing token');
  });

  it('should not log GITHUB_TOKEN in any output', async () => {
    mockResponses = [
      { status: 401, statusText: 'Unauthorized', body: '{ "message": "Bad credentials" }' }
    ];

    const { scheduled } = await import('./index.js');
    const env = { GITHUB_TOKEN: 'ghp_secrettoken123' };
    await scheduled(env, { waitUntil: fn => fn() });

    const logs = parseLogs();
    logs.forEach(log => {
      if (typeof log === 'object') {
        ok(!JSON.stringify(log).includes('ghp_secrettoken123'), `Token should not appear in log: ${JSON.stringify(log)}`);
      }
    });
  });
});

describe('http handler', () => {
  beforeEach(() => {
    resetMocks();
  });

  it('should return 404 for non-GET requests', async () => {
    const request = new Request('https://example.com/ping', { method: 'POST' });
    const { fetch: httpHandler } = await import('./index.js');
    const response = await httpHandler(request, {});
    strictEqual(response.status, 404);
  });

  it('should return status info for GET /ping', async () => {
    const request = new Request('https://example.com/ping', { method: 'GET' });
    const { fetch: httpHandler } = await import('./index.js');
    const response = await httpHandler(request, {});
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
    const { fetch: httpHandler } = await import('./index.js');
    const response = await httpHandler(request, {});
    const body = await response.json();
    strictEqual(body.hasToken, true);
  });
});

describe('utilities', () => {
  it('should parse Retry-After header', async () => {
    const response = { headers: { get: (name) => name === 'Retry-After' ? '5' : null } };
    const { getRetryAfterSeconds } = await import('./index.js');
    strictEqual(getRetryAfterSeconds(response), 5);
  });

  it('should default Retry-After to 0 when missing', async () => {
    const response = { headers: { get: () => null } };
    const { getRetryAfterSeconds } = await import('./index.js');
    strictEqual(getRetryAfterSeconds(response), 0);
  });
});