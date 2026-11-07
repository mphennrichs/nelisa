// Proxy de /api/* para a API Go na mesma origem (ADR 0004): o navegador só
// fala com o Next, então não há CORS e o cookie de refresh fica
// SameSite=Strict no domínio do Nelisa. Nenhuma regra de negócio aqui.
import type { NextRequest } from "next/server";

export const dynamic = "force-dynamic";

// Cabeçalhos que não atravessam um proxy (RFC 9110 §7.6.1) ou que o fetch
// recalcula.
const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "host",
  "content-length",
]);

function apiBase(): string {
  const base = process.env.API_INTERNAL_URL;
  if (!base) throw new Error("API_INTERNAL_URL não definido");
  return base.replace(/\/$/, "");
}

async function proxy(req: NextRequest, ctx: { params: Promise<{ path: string[] }> }) {
  const { path } = await ctx.params;
  const target = new URL(`${apiBase()}/api/${path.map(encodeURIComponent).join("/")}`);
  target.search = req.nextUrl.search;

  const headers = new Headers();
  req.headers.forEach((value, key) => {
    if (!HOP_BY_HOP.has(key.toLowerCase())) headers.set(key, value);
  });

  const hasBody = req.method !== "GET" && req.method !== "HEAD";
  let upstream: Response;
  try {
    upstream = await fetch(target, {
      method: req.method,
      headers,
      body: hasBody ? await req.arrayBuffer() : undefined,
      redirect: "manual",
      cache: "no-store",
    });
  } catch {
    return Response.json(
      { error: { code: "api_unreachable", message: "Serviço indisponível. Tente de novo em instantes." } },
      { status: 502 },
    );
  }

  const out = new Headers();
  upstream.headers.forEach((value, key) => {
    if (!HOP_BY_HOP.has(key.toLowerCase()) && key.toLowerCase() !== "set-cookie") out.set(key, value);
  });
  // Set-Cookie pode vir repetido; Headers.forEach junta os valores.
  for (const cookie of upstream.headers.getSetCookie()) out.append("set-cookie", cookie);

  return new Response(upstream.body, { status: upstream.status, headers: out });
}

export { proxy as GET, proxy as POST, proxy as PUT, proxy as PATCH, proxy as DELETE };
