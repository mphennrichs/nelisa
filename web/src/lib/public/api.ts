// Leitura da camada pública da API, só no servidor (Server Components).
// A regra de visibilidade é da API Go: aqui só buscamos e mostramos.
import type { components } from "@/lib/api/schema";

export type PublicProposition = components["schemas"]["PublicProposition"];
export type PublicPropositionDetail = components["schemas"]["PublicPropositionDetail"];

function base() {
  return (process.env.API_INTERNAL_URL ?? "http://localhost:8080").replace(/\/$/, "");
}

export class PublicApiError extends Error {}

async function get<T>(path: string): Promise<T | null> {
  let res: Response;
  try {
    res = await fetch(`${base()}${path}`, { next: { revalidate: 60 } });
  } catch {
    throw new PublicApiError("API indisponível");
  }
  if (res.status === 404) return null;
  if (!res.ok) throw new PublicApiError(`API respondeu ${res.status}`);
  return (await res.json()) as T;
}

export async function listPublicPropositions(): Promise<PublicProposition[]> {
  const r = await get<{ propositions: PublicProposition[] }>("/api/public/propositions");
  return r?.propositions ?? [];
}

export function getPublicProposition(slug: string) {
  return get<PublicPropositionDetail>(`/api/public/propositions/${encodeURIComponent(slug)}`);
}

export type ContributionAvailability = { open: boolean; reason: string | null };

/** Se o formulário de Contribuição está aberto. Sem cache: liga e desliga na hora. */
export async function getContributionAvailability(): Promise<ContributionAvailability> {
  try {
    const res = await fetch(`${base()}/api/public/contributions/availability`, { cache: "no-store" });
    if (!res.ok) return { open: false, reason: "unavailable" };
    return (await res.json()) as ContributionAvailability;
  } catch {
    return { open: false, reason: "unavailable" };
  }
}
