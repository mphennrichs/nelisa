import Link from "next/link";
import { notFound } from "next/navigation";
import * as A from "@/app/actions";
import { TOPICS, propositionLabel, chamberOf, claimOf, db, userById } from "@/lib/store";
import { requireTeamUser } from "@/lib/session";
import { TEAM_STATE_LABEL, fmtDate } from "@/lib/format";
import { Badge, ErrorBanner, Hidden } from "@/components/ui";

export default async function EditPropositionPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ erro?: string }>;
}) {
  const u = await requireTeamUser();
  const { id } = await params;
  const { erro } = await searchParams;
  const proposition = db.propositions.find((b) => b.id === id);
  if (!proposition) notFound();

  const claim = claimOf(proposition);
  const mine = claim?.userId === u.id;
  const canApprove = u.role !== "editor";
  const B = <Hidden name="propositionId" value={proposition.id} />;

  return (
    <div className="space-y-5">
      <header className="space-y-1">
        <div className="flex items-center gap-2 text-sm text-stone-500">
          <span>{chamberOf(proposition).name}</span>
          <Badge tone={proposition.state === "ready" ? "green" : "amber"}>{TEAM_STATE_LABEL[proposition.state]}</Badge>
          {proposition.pendingSecondReview && <Badge tone="blue">Segunda revisão pendente</Badge>}
          {proposition.aiDraftUsed && <Badge>Partiu de rascunho de IA</Badge>}
        </div>
        <h1 className="text-2xl font-bold">{propositionLabel(proposition)}</h1>
        <p className="text-stone-600">{proposition.ementa}</p>
        <p className="text-sm">
          <Link href={`/proposicao/${proposition.id}`} className="link">Ver como o leitor vê</Link> ·{" "}
          <a href={proposition.version.officialUrl} className="link" target="_blank" rel="noreferrer">Texto oficial ↗</a>
        </p>
      </header>

      <ErrorBanner message={erro} />

      <section className="card flex flex-wrap items-center gap-3 text-sm">
        <span className="font-semibold">Posse:</span>
        {claim ? (
          <span>{userById(claim.userId)?.name} (última atividade {fmtDate(claim.lastActivity)}, expira em 3 dias sem atividade)</span>
        ) : (
          <span className="text-stone-500">livre</span>
        )}
        <span className="flex-1" />
        {!claim && proposition.state !== "ready" && (
          <form action={A.claimProposition}>{B}<button className="btn">Assumir</button></form>
        )}
        {claim && (mine || canApprove) && (
          <form action={A.releaseClaim}>{B}<button className="btn-ghost">Liberar posse</button></form>
        )}
      </section>

      <section className="card space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">Resumo, resumo curto e temas</h2>
          <form action={A.aiDraft}>{B}<button className="btn-ghost" disabled={!mine}>✨ Gerar rascunho de IA (simulado)</button></form>
        </div>
        <form action={A.saveSummary} className="space-y-3">
          {B}
          <textarea name="summary" defaultValue={proposition.summary} rows={4} className="input" placeholder="Resumo em linguagem acessível" disabled={!mine} />
          <input name="shortSummary" defaultValue={proposition.shortSummary} maxLength={280} className="input" placeholder="Resumo curto (até 280 caracteres, vai na imagem de story)" disabled={!mine} />
          <div className="flex flex-wrap gap-3 text-sm">
            {TOPICS.map((t) => (
              <label key={t} className="flex items-center gap-1">
                <input type="checkbox" name="topics" value={t} defaultChecked={proposition.topics.includes(t)} disabled={!mine} /> {t}
              </label>
            ))}
          </div>
          <div className="flex items-center gap-3">
            <button className="btn" disabled={!mine}>Salvar</button>
            {!mine && <span className="text-xs text-stone-500">Assuma a proposição para editar.</span>}
            {proposition.summaryAuthorIds.length > 0 && <span className="text-xs text-stone-500">Resumo escrito por {proposition.summaryAuthorIds.map((id) => userById(id)?.name).join(", ")}</span>}
          </div>
        </form>
      </section>

      <section className="space-y-2">
        <h2 className="font-semibold">Dispositivos e comentários (públicos quando pronto)</h2>
        {proposition.version.provisions.map((p) => (
          <div key={p.id} className="grid gap-3 rounded-lg border border-stone-200 bg-white p-3 md:grid-cols-[1fr_320px]">
            <div>
              <div className="text-xs font-semibold uppercase text-stone-500">{p.label}</div>
              <p>{p.text}</p>
            </div>
            <div className="space-y-2">
              {proposition.version.comments.filter((c) => c.provisionId === p.id).map((c) => (
                <p key={c.id} className="rounded-md bg-yellow-50 p-2 text-sm">
                  {c.text} <span className="text-xs text-stone-500">— {userById(c.authorId)?.name}</span>
                </p>
              ))}
              {mine && (
                <form action={A.addComment} className="flex gap-1">
                  {B}
                  <Hidden name="provisionId" value={p.id} />
                  <input name="text" className="input" placeholder="Comentar este dispositivo" />
                  <button className="btn-ghost">+</button>
                </form>
              )}
            </div>
          </div>
        ))}
      </section>

      <section className="card space-y-2">
        <h2 className="font-semibold">Notas internas (nunca públicas)</h2>
        {proposition.internalNotes.map((n) => (
          <p key={n.id} className="text-sm">
            <span className="text-stone-500">{fmtDate(n.at)} · {userById(n.authorId)?.name}:</span> {n.text}
          </p>
        ))}
        <form action={A.addInternalNote} className="flex gap-2">
          {B}
          <input name="text" className="input" placeholder="Escreva para a equipe" />
          <button className="btn-ghost">Enviar</button>
        </form>
      </section>

      {canApprove && (
        <section className="card flex flex-wrap items-center gap-3">
          <h2 className="font-semibold">Aprovação</h2>
          <span className="text-sm text-stone-500">
            Quatro olhos: quem escreveu o resumo não aprova (exceto Admin, que fica marcado como “revisado por 1 pessoa”).
          </span>
          <span className="flex-1" />
          {proposition.state !== "ready" && <form action={A.markReady}>{B}<button className="btn">Marcar como pronto</button></form>}
          {proposition.pendingSecondReview && (
            <form action={A.confirmSecondReview}>{B}<button className="btn">Confirmar segunda revisão</button></form>
          )}
        </section>
      )}

      {proposition.approvals.length > 0 && (
        <p className="text-xs text-stone-500">
          Aprovações: {proposition.approvals.map((a) => `${userById(a.userId)?.name} em ${fmtDate(a.at)}${a.bypass ? " (sem quatro olhos)" : ""}`).join("; ")}
        </p>
      )}
    </div>
  );
}
