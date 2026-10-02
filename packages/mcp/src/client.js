export class ApiError extends Error {
  constructor(status, body) {
    super(typeof body?.error === 'string' ? body.error : `HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

/** Calls the Go-Split API with a bearer token minted by POST /auth/tokens. */
export function createClient({ baseUrl, token, fetch = globalThis.fetch }) {
  const root = baseUrl.replace(/\/+$/, '');
  return async function request(method, path, body) {
    const response = await fetch(root + path, {
      method,
      headers: {
        Authorization: `Bearer ${token}`,
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    const data = text ? JSON.parse(text) : null;
    if (!response.ok) throw new ApiError(response.status, data);
    return data;
  };
}
