import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import PublicShell, { UnderReviewBadge } from "@/components/PublicShell";
import { getPublicProposition, type PublicPropositionDetail } from "@/lib/public/api";
import { chamberLabel, fmtDate, officialText, chamberIn } from "@/lib/labels";

// Renderiza sob demanda (a API não existe no build da imagem); os dados
// ficam 60s no cache do fetch (lib/public/api.ts).
export const dynamic = "force-dynamic";

const RECENT = 20;

type Entry = PublicPropositionDetail["timeline"][number];

function Timeline({ entries }: { entries: Entry[] }) {
  return (
    <ol className="space-y-3">
      {entries.map((e, i) => (
        <li key={`${e.chamber}-${e.at}-${i}`} className="border-l-2 border-stone-300 pl-4">
          <p className="text-sm text-stone-600">
            <time dateTime={e.at}>{fmtDate(e.at)}</time> · {chamberLabel(e.chamber)}
            {e.body && <> · {e.body}</>}
          </p>
          <p className="font-medium">{e.description}</p>
          {e.dispatch && e.dispatch !== e.description && <p className="text-stone-700 [overflow-wrap:anywhere]">{e.dispatch}</p>}
        </li>
      ))}
    </ol>
  );
}

type Props = { params: Promise<{ slug: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const p = await getPublicProposition((await params).slug);
  if (!p) return { title: "Proposição não encontrada · Nelisa" };
  const description = p.official_summary.length > 200 ? `${p.official_summary.slice(0, 197)}…` : p.official_summary;
  return {
    title: `${p.name} · Nelisa`,
    description,
    openGraph: { title: p.name, description, type: "article", locale: "pt_BR", siteName: "Nelisa" },
  };
}

export default async function PropositionPage({ params }: Props) {
  const p = await getPublicProposition((await params).slug);
  if (!p) notFound();
  const timeline = [...p.timeline].reverse(); // mais recente primeiro

  return (
    <PublicShell>
      <article className="space-y-8">
        <header className="space-y-3">
          <div className="flex flex-wrap items-center gap-2 text-sm text-stone-600">
            <span>Iniciada {chamberIn(p.origin_chamber)}</span>
            <span aria-hidden="true">·</span>
            <span>apresentada em {fmtDate(p.presented_at)}</span>
            {p.under_review && <UnderReviewBadge />}
          </div>
          <h1 className="text-3xl font-bold text-balance sm:text-4xl">{p.name}</h1>
          <p className="text-lg leading-relaxed text-stone-800">{p.official_summary}</p>
          {p.under_review && (
            <p className="rounded-md border border-sky-200 bg-sky-50 px-3 py-2 text-sm text-sky-950">
              Em análise pela equipe: por enquanto mostramos só os dados oficiais. O resumo e os comentários do Nelisa
              aparecem depois da revisão.
            </p>
          )}
        </header>

        <section aria-labelledby="dados" className="card space-y-3">
          <h2 id="dados" className="text-lg font-semibold">Dados oficiais</h2>
          <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-[max-content_1fr]">
            <dt className="font-medium text-stone-600">Situação</dt>
            <dd>
              {officialText(p.status) || "—"}
              {p.superseded && <span className="ml-2 text-sm text-stone-600">(prejudicada)</span>}
            </dd>
            {p.authors.length > 0 && (
              <>
                <dt className="font-medium text-stone-600">Autoria</dt>
                <dd>{p.authors.length > 6 ? `${p.authors.slice(0, 6).join(", ")} e mais ${p.authors.length - 6}` : p.authors.join(", ")}</dd>
              </>
            )}
            {p.attached_to && (
              <>
                <dt className="font-medium text-stone-600">Apensada a</dt>
                <dd>{p.attached_to.slug ? <Link href={`/proposicao/${p.attached_to.slug}`} className="link">{p.attached_to.name}</Link> : p.attached_to.name}</dd>
              </>
            )}
            {p.attached.length > 0 && (
              <>
                <dt className="font-medium text-stone-600">Apensadas</dt>
                <dd>
                  {p.attached.map((a, i) => (
                    <span key={a.name}>
                      {i > 0 && ", "}
                      {a.slug ? <Link href={`/proposicao/${a.slug}`} className="link">{a.name}</Link> : a.name}
                    </span>
                  ))}
                </dd>
              </>
            )}
            {p.official_text_url && (
              <>
                <dt className="font-medium text-stone-600">Documento integral</dt>
                <dd><a href={p.official_text_url} className="link" rel="noopener noreferrer">Texto oficial na Casa</a></dd>
              </>
            )}
            {p.sources.length > 0 && (
              <>
                <dt className="font-medium text-stone-600">Fontes</dt>
                <dd className="space-y-1">
                  {p.sources.map((s) => (
                    <a key={`${s.chamber}-${s.url}`} href={s.url} className="link block" rel="noopener noreferrer">
                      {s.name} {chamberIn(s.chamber)}
                    </a>
                  ))}
                </dd>
              </>
            )}
          </dl>
        </section>

        <section aria-labelledby="tramitacao" className="space-y-3">
          <h2 id="tramitacao" className="text-lg font-semibold">Tramitação ({timeline.length} eventos)</h2>
          {timeline.length === 0 ? (
            <p className="text-stone-700">Sem eventos de tramitação ainda.</p>
          ) : (
            <>
              <Timeline entries={timeline.slice(0, RECENT)} />
              {timeline.length > RECENT && (
                <details className="group">
                  <summary className="btn-ghost min-h-11 cursor-pointer px-4">
                    Ver os {timeline.length - RECENT} eventos anteriores
                  </summary>
                  <div className="mt-3">
                    <Timeline entries={timeline.slice(RECENT)} />
                  </div>
                </details>
              )}
            </>
          )}
        </section>
      </article>
    </PublicShell>
  );
}
