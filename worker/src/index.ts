export interface Env {
    STORE: KVNamespace;
    AUTH_TOKEN?: string;
}

export default {
    async fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
        const url = new URL(request.url);

        if (url.pathname !== '/v1/store') {
            return new Response('Not Found', { status: 404 });
        }

        if (env.AUTH_TOKEN) {
            const auth = request.headers.get('Authorization');
            if (!auth || auth !== `Bearer ${env.AUTH_TOKEN}`) {
                return new Response('Unauthorized', { status: 401 });
            }
        }

        const STORE_KEY = 'envsync_state';

        if (request.method === 'GET') {
            const value = await env.STORE.get(STORE_KEY);
            if (!value) {
                return new Response('null', { status: 200, headers: { 'Content-Type': 'application/json' } });
            }
            return new Response(value, { status: 200, headers: { 'Content-Type': 'application/json' } });
        }

        if (request.method === 'PUT') {
            try {
                const currentStr = await env.STORE.get(STORE_KEY);
                const currentRev = currentStr ? JSON.parse(currentStr).revision || 0 : 0;

                const expectedRevStr = request.headers.get('If-Match');
                const expectedRev = expectedRevStr ? parseInt(expectedRevStr, 10) : 0;

                if (currentRev !== expectedRev) {
                    return new Response(`Conflict: expected revision ${expectedRev}, got ${currentRev}`, { status: 409 });
                }

                const body = await request.text();
                await env.STORE.put(STORE_KEY, body);

                return new Response('OK', { status: 200 });
            } catch (err: any) {
                return new Response(`Bad Request: ${err.message}`, { status: 400 });
            }
        }

        return new Response('Method Not Allowed', { status: 405 });
    },
};
