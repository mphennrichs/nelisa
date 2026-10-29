import Link from "next/link";
import { notFound } from "next/navigation";
import type { Metadata } from "next";
import { chambersOf, db, propositionLabel, toPublic } from "@/lib/store";
import { PROGRESS_LABEL, PUBLIC_STATE_LABEL, SPHERE_LABEL, daysUntil, fmtDate } from "@/lib/format";
import { Badge } from "@/components/ui";
import ShareStory from "./ShareStory";
import type { RollCallVote } from "@/lib/types";

type Params = Promise<{ id: string }>;

function load(id: string) {
  const proposition = db.propositions.find((b) => b.id === id);
  return proposition ? toPublic(proposition) : null;
}

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const b = load((await params).id);
  if (!b) return {};
  return { title: `${b.kind} ${b.number}/${b.year} — Nelisa`, description: b.shortSummary ?? b.ementa };
}

export default async function PropositionPage({ params }: { params: Params }) {
  const b = load((await params).id);
  if (!b) notFound();
  const chambers = chambersOf(b);
  const principal = b.attachedTo ? db.propositions.find((p) => p.id === b.attachedTo) : undefined;
  const attached = db.propositions.filter((p) => p.attachedTo === b.id && p.state !== "draft");
  const ready = b.state === "ready";
  const nextVote = b.progress.find((p) => p.kind === "vote_scheduled" && daysUntil(p.date) >= 0);

  return (
    <article className="space-y-6">
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-2 text-sm text-stone-500">
          <Badge>{SPHERE_LABEL[chambers[0].sphere]}</Badge>
          <span>{chambers.map((c) => c.name).join(" → ")}</span>
          <Badge tone={ready ? "green" : "amber"}>{PUBLIC_STATE_LABEL[b.state]}</Badge>
          {ready && <span>· revisado por {b.reviewers} {b.reviewers === 1 ? "pessoa" : "pessoas"}</span>}
        </div>
        <h1 className="text-3xl font-bold">{b.kind} {b.number}/{b.year}</h1>
        <p className="text-stone-600"><span className="font-semibold">Ementa oficial:</span> {b.ementa}</p>
        <p className="text-sm">
          <span className="font-semibold">Autoria: </span>
          {b.authors.map((a, i) => (
            <span key={i}>
              {i > 0 && ", "}
              {a.kind === "legislator" ? (
                <AuthorLink id={a.legislatorId} chamberId={b.chamberIds[0]} at={b.progress[0]?.date ?? b.createdAt} />
              ) : a.kind === "cosigners" ? (
                `e mais ${a.count} coautores`
              ) : (
                a.name
              )}
            </span>
          ))}
        </p>
        <p className="flex flex-wrap gap-3 text-sm">
          <a href={b.officialUrl} className="link" target="_blank" rel="noreferrer">Texto oficial na fonte ↗</a>
          <Link href={`/contribuir?tipo=progress_update&proposicao=${b.id}`} className="link">Informar mudança na tramitação</Link>
        </p>
        {principal && (
          <div className="rounded-md bg-stone-100 px-3 py-2 text-sm">
            {b.superseded ? "Prejudicada: deixou de tramitar sem votação. " : ""}Tramitou apensada à{" "}
            <Link href={`/proposicao/${principal.id}`} className="link">{propositionLabel(principal)}</Link>.
          </div>
        )}
        {attached.length > 0 && (
          <div className="rounded-md bg-stone-100 px-3 py-2 text-sm">
            Apensadas a esta:{" "}
            {attached.map((p, i) => (
              <span key={p.id}>{i > 0 && ", "}<Link href={`/proposicao/${p.id}`} className="link">{propositionLabel(p)}</Link></span>
            ))}
          </div>
        )}
        {nextVote && (
          <div className="rounded-md bg-sky-50 px-3 py-2 text-sm text-sky-900">
            {nextVote.description}: {fmtDate(nextVote.date)} (em {daysUntil(nextVote.date)} dias)
          </div>
        )}
      </header>

      {!ready && (
        <div className="card border-amber-300 bg-amber-50 text-amber-900">
          <strong>Em análise.</strong> A equipe ainda está revisando esta proposição. Enquanto isso, você vê só os dados oficiais
          e o link para o texto na fonte.
        </div>
      )}

      {ready && (
        <section className="card space-y-3">
          <div className="flex flex-wrap gap-1">{b.topics.map((t) => <Badge key={t} tone="blue">{t}</Badge>)}</div>
          <h2 className="text-lg font-semibold">Resumo</h2>
          <p className="leading-relaxed">{b.summary}</p>
          <ShareStory text={b.shortSummary!} url={`nelisa.org/proposicao/${b.id}`} />
        </section>
      )}

      {ready && b.provisions && (
        <section className="space-y-2">
          <h2 className="text-lg font-semibold">Texto comentado</h2>
          {b.provisions.map((p) => {
            const comments = b.comments!.filter((c) => c.provisionId === p.id);
            return (
              <div key={p.id} className="grid gap-3 rounded-lg border border-stone-200 bg-white p-3 md:grid-cols-[1fr_280px]">
                <div>
                  <div className="text-xs font-semibold uppercase text-stone-500">{p.label}</div>
                  <p>{p.text}</p>
                </div>
                <div className="space-y-2">
                  {comments.map((c, i) => (
                    <p key={i} className="rounded-md bg-yellow-50 p-2 text-sm text-stone-800">💬 {c.text}</p>
                  ))}
                </div>
              </div>
            );
          })}
        </section>
      )}

      <section className="card">
        <h2 className="mb-3 text-lg font-semibold">Tramitação</h2>
        <ol className="space-y-2 border-l-2 border-stone-200 pl-4">
          {b.progress.map((p) => (
            <li key={p.id}>
              <span className="text-sm text-stone-500">{fmtDate(p.date)} · {PROGRESS_LABEL[p.kind]}</span>
              <div>{p.description}</div>
            </li>
          ))}
        </ol>
        {b.rollCall && <RollCall {...b.rollCall} />}
      </section>
    </article>
  );
}

function RollCall({ description, votes }: { description: string; votes: RollCallVote[] }) {
  const groups = new Map<string, RollCallVote[]>();
  for (const v of votes) groups.set(v.vote, [...(groups.get(v.vote) ?? []), v]);
  const order = [...groups.keys()].sort((a, b) => groups.get(b)!.length - groups.get(a)!.length);
  return (
    <details className="mt-4">
      <summary className="cursor-pointer font-semibold">
        Votos nominais: {description} ({order.map((k) => `${k}: ${groups.get(k)!.length}`).join(" · ")})
      </summary>
      <p className="mt-1 text-xs text-stone-500">Como publicado pela fonte oficial, com os rótulos dela.</p>
      <div className="mt-2 grid gap-3 sm:grid-cols-2">
        {order.map((k) => (
          <div key={k}>
            <h4 className="text-sm font-semibold">{k} ({groups.get(k)!.length})</h4>
            <ul className="text-sm text-stone-700">
              {groups.get(k)!.map((v) => (
                <li key={v.name}>{v.name} ({v.party}/{v.uf})</li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </details>
  );
}

// Party is the one of the Term in force when the proposition was presented.
function AuthorLink({ id, chamberId, at }: { id: string; chamberId: string; at: string }) {
  const l = db.legislators.find((x) => x.id === id)!;
  const day = at.slice(0, 10);
  const term = l.terms.find((t) => t.start <= day && (!t.end || t.end >= day)) ?? l.terms.find((t) => t.chamberId === chamberId);
  return (
    <Link href={`/parlamentar/${l.id}`} className="link">
      {l.name}{term && ` (${term.party}/${term.uf})`}
    </Link>
  );
}
