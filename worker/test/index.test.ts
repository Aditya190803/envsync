import { describe, it, expect, vi } from 'vitest';
import worker from '../src/index';

function makeEnv(store: any, token?: string) {
  return {
    STORE: store,
    AUTH_TOKEN: token,
  } as any;
}

describe('worker fetch handler', () => {
  it('GET returns null body when store is empty', async () => {
    const store = { get: vi.fn().mockResolvedValue(null), put: vi.fn() };
    const req = new Request('https://example.test/v1/store', { method: 'GET', headers: { Authorization: 'Bearer secret' } });

    const res = await (worker as any).fetch(req, makeEnv(store, 'secret'), {} as any);
    expect(res.status).toBe(200);
    const text = await res.text();
    expect(text).toBe('null');
    expect(store.get).toHaveBeenCalled();
  });

  it('returns 401 when AUTH_TOKEN set and missing header', async () => {
    const store = { get: vi.fn().mockResolvedValue(null), put: vi.fn() };
    const req = new Request('https://example.test/v1/store', { method: 'GET' });

    const res = await (worker as any).fetch(req, makeEnv(store, 'secret'), {} as any);
    expect(res.status).toBe(401);
  });

  it('PUT stores value when revision matches', async () => {
    const initial = JSON.stringify({ revision: 0, data: {} });
    const store = { get: vi.fn().mockResolvedValue(initial), put: vi.fn().mockResolvedValue(undefined) };

    const body = JSON.stringify({ revision: 1, data: { foo: 'bar' } });
    const req = new Request('https://example.test/v1/store', {
      method: 'PUT',
      headers: { 'If-Match': '0' },
      body,
    } as any);

    const res = await (worker as any).fetch(req, makeEnv(store), {} as any);
    expect(res.status).toBe(200);
    expect(store.put).toHaveBeenCalledWith('envsync_state', body);
  });

  it('PUT returns 409 on revision conflict', async () => {
    const initial = JSON.stringify({ revision: 5 });
    const store = { get: vi.fn().mockResolvedValue(initial), put: vi.fn() };

    const body = JSON.stringify({ revision: 6 });
    const req = new Request('https://example.test/v1/store', {
      method: 'PUT',
      headers: { 'If-Match': '0' },
      body,
    } as any);

    const res = await (worker as any).fetch(req, makeEnv(store), {} as any);
    expect(res.status).toBe(409);
  });
});
