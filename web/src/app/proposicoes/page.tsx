import type { Metadata } from "next";
import Link from "next/link";
import PublicShell, { UnderReviewBadge } from "@/components/PublicShell";
import { listPublicPropositions } from "@/lib/public/api";
import { chamberLabel, fmtDate, officialText } from "@/lib/labels";

export const metadata: Metadata = {
  title: "Proposições · Nelisa",
  description: "Proposições legislativas acompanhadas pelo Nelisa.",
};

// Renderiza sob demanda (a API não existe no build da imagem); os dados
// ficam 60s no cache do fetch (lib/public/api.ts).
export const dynamic = "force-dynamic";

export default async function PropositionsPage() {
  const items = await listPublicPropositions();
  return (
    <PublicShell>
      <h1 className="mb-6 text-3xl font-bold">Proposições</h1>
      {items.length === 0 ? (
        <p className="card text-stone-700">Ainda não há Proposições publicadas. Volte em breve.</p>
      ) : (
        <ul className="space-y-4">
          {items.map((p) => (
            <li key={p.slug} className="card space-y-2">
              <div className="flex flex-wrap items-center gap-2 text-sm text-stone-600">
                <span>{chamberLabel(p.origin_chamber)}</span>
                <span aria-hidden="true">·</span>
                <span>apresentada em {fmtDate(p.presented_at)}</span>
                {p.under_review && <UnderReviewBadge />}
              </div>
              <h2 className="text-xl font-semibold">
                <Link href={`/proposicao/${p.slug}`} className="hover:underline">{p.name}</Link>
              </h2>
              <p className="leading-relaxed text-stone-800">{p.summary}</p>
              {p.status && <p className="text-sm text-stone-600">Situação: {officialText(p.status)}</p>}
            </li>
          ))}
        </ul>
      )}
    </PublicShell>
  );
}
