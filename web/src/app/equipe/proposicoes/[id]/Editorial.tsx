"use client";

// Seção "Conteúdo" da Proposição na equipe: Versões, Posse, Resumo, Resumo
// curto, Temas, aprovação e Selo. Toda decisão de permissão vem da API
// (can_*, approve_blockers); aqui só mostramos e chamamos.
import { useEffect, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FieldError, FormError } from "@/lib/team/ui";
import { VERSION_STATE_LABEL, charCount, fmtDateTime, paragraphs, reviewBadgeLabel } from "@/lib/labels";

type Detail = components["schemas"]["TeamPropositionDetail"];
type Version = components["schemas"]["PropositionVersion"];
type Content = components["schemas"]["VersionContent"];
type Topic = components["schemas"]["Topic"];

// Limites do contrato (openapi: VersionContent, CreateVersionRequest).
const SUMMARY_MAX = 5000;
const SHORT_MAX = 280;
const TOPICS_MAX = 3;
const LABEL_MAX = 120;

type Feedback = { area: string; kind: "ok" | "error"; message: string };

function errMessage(e: unknown, fallback: string) {
  return e instanceof ApiError ? e.message : fallback;
}

function Notice({ feedback, area }: { feedback?: Feedback; area: string }) {
  if (!feedback || feedback.area !== area) return null;
  if (feedback.kind === "error") return <FormError message={feedback.message} />;
  return (
    <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">
      {feedback.message}
    </p>
  );
}

function Paragraphs({ text, empty }: { text: string; empty: string }) {
  const ps = paragraphs(text);
  if (ps.length === 0) return <p className="text-stone-600">{empty}</p>;
  return (
    <div className="space-y-3">
      {ps.map((p, i) => (
        <p key={i} className="whitespace-pre-line leading-relaxed [overflow-wrap:anywhere]">{p}</p>
      ))}
    </div>
  );
}

function names(ids: string[], people: Record<string, string>) {
  return ids.map((id) => people[id] ?? "pessoa removida").join(", ");
}

export default function Editorial({ p, reload }: { p: Detail; reload: () => Promise<Detail | undefined> }) {
  const { api } = useTeamSession();
  const [feedback, setFeedback] = useState<Feedback>();
  const [busy, setBusy] = useState<string>();
  const [editorDirty, setEditorDirty] = useState(false);
  const [confirmApprove, setConfirmApprove] = useState(false);
  const [confirmRelease, setConfirmRelease] = useState(false);
  const [newVersionOpen, setNewVersionOpen] = useState(false);

  const base = `/api/team/propositions/${encodeURIComponent(p.id)}`;
  const pending = p.versions.find((v) => v.state === "pending");
  const current = p.versions.find((v) => v.state === "current");
  const topicsById = new Map(p.topics.map((t) => [t.id, t]));
  const versions = [...p.versions].sort((a, b) => b.seq - a.seq);

  async function run(area: string, key: string, fn: () => Promise<unknown>, done: string) {
    setBusy(key);
    setFeedback(undefined);
    try {
      await fn();
      setFeedback({ area, kind: "ok", message: done });
      await reload();
      return true;
    } catch (e) {
      setFeedback({ area, kind: "error", message: errMessage(e, "A ação falhou. Tente de novo.") });
      await reload();
      return false;
    } finally {
      setBusy(undefined);
    }
  }

  const claim = p.claim;
  const approval = p.current_approval;

  return (
    <section aria-labelledby="conteudo" className="card space-y-8">
      <h2 id="conteudo" className="text-lg font-semibold">Conteúdo</h2>

      {/* Selo e aprovação da Versão vigente */}
      <div className="space-y-2">
        <h3 className="font-semibold">Selo de revisão</h3>
        {p.review_badge == null || !approval ? (
          <p className="text-stone-700">Nenhuma Versão aprovada ainda: o leitor vê só os dados oficiais.</p>
        ) : (
          <>
            <p>
              <span className="inline-block rounded bg-stone-100 px-2 py-0.5 text-sm font-medium text-stone-800">
                {reviewBadgeLabel(p.review_badge)}
              </span>
            </p>
            <p className="text-sm text-stone-700">
              Versão {current?.seq} aprovada por {p.people[approval.approved_by] ?? "pessoa removida"} em{" "}
              {fmtDateTime(approval.approved_at)}
              {approval.four_eyes && " (quatro olhos: quem aprovou não escreveu)"}.
              {approval.second_review_by &&
                ` Segunda revisão de ${p.people[approval.second_review_by] ?? "pessoa removida"} em ${fmtDateTime(approval.second_review_at)}.`}
            </p>
            {approval.pending_second_review && (
              <p className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-950">
                Segunda revisão pendente: quem aprovou também escreveu o conteúdo. Outra pessoa (Revisor ou Admin) que não
                escreveu precisa confirmar a revisão para o Selo passar a 2.
              </p>
            )}
            {p.can_second_review && (
              <button
                type="button"
                className="btn-lg"
                disabled={busy !== undefined}
                onClick={() =>
                  run(
                    "seal",
                    "second",
                    () => api(`${base}/versions/${encodeURIComponent(approval.version_id)}/second-review`, { method: "POST" }),
                    "Segunda revisão confirmada. O Selo agora é “revisado por 2 pessoas”.",
                  )
                }
              >
                {busy === "second" ? "Confirmando…" : "Confirmar segunda revisão"}
              </button>
            )}
          </>
        )}
        <Notice feedback={feedback} area="seal" />
      </div>

      {/* Versões */}
      <div className="space-y-3 border-t border-stone-200 pt-6">
        <h3 className="font-semibold">Versões do texto</h3>
        {versions.length === 0 ? (
          <p className="text-stone-700">Nenhuma Versão registrada.</p>
        ) : (
          <ol className="space-y-3">
            {versions.map((v: Version) => (
              <li key={v.id} className="space-y-1">
                <p className="flex flex-wrap items-center gap-2">
                  <span className="font-medium [overflow-wrap:anywhere]">
                    Versão {v.seq}: {v.label}
                  </span>
                  <span
                    className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                      v.state === "pending"
                        ? "bg-sky-100 text-sky-900"
                        : v.state === "current"
                          ? "bg-emerald-100 text-emerald-900"
                          : "bg-stone-100 text-stone-700"
                    }`}
                  >
                    {VERSION_STATE_LABEL[v.state]}
                  </span>
                </p>
                <p className="text-sm text-stone-600">
                  {v.official_text_url && (
                    <>
                      <a href={v.official_text_url} className="link" rel="noopener noreferrer">
                        Documento integral
                      </a>
                      {" · "}
                    </>
                  )}
                  registrada em {fmtDateTime(v.created_at)}{" "}
                  {v.created_by ? `por ${p.people[v.created_by] ?? "pessoa removida"}` : "pela Importação"}
                </p>
              </li>
            ))}
          </ol>
        )}

        {p.can_create_version && !newVersionOpen && (
          <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setNewVersionOpen(true)}>
            Registrar nova Versão
          </button>
        )}
        {p.can_create_version && newVersionOpen && (
          <NewVersionForm
            base={base}
            onCancel={() => setNewVersionOpen(false)}
            onDone={async () => {
              setNewVersionOpen(false);
              setFeedback({
                area: "versions",
                kind: "ok",
                message: "Versão registrada, em análise. O conteúdo da vigente foi trazido para ela: confira e confirme.",
              });
              await reload();
            }}
          />
        )}
        <Notice feedback={feedback} area="versions" />
      </div>

      {/* Posse */}
      {(pending || claim) && (
        <div className="space-y-3 border-t border-stone-200 pt-6">
          <h3 className="font-semibold">Posse</h3>
          {!claim && <p className="text-stone-700">Ninguém está com esta Proposição. Quem assume a Posse edita o conteúdo.</p>}
          {claim?.mine && (
            <p className="text-stone-700">
              Você está com a Posse desde {fmtDateTime(claim.claimed_at)}. Ela expira em {fmtDateTime(claim.expires_at)} se não
              houver atividade.
            </p>
          )}
          {claim && !claim.mine && (
            <p className="text-stone-700">
              Com <strong>{claim.holder_name}</strong> desde {fmtDateTime(claim.claimed_at)} (última atividade em{" "}
              {fmtDateTime(claim.last_activity_at)}; expira em {fmtDateTime(claim.expires_at)}). Enquanto isso, só essa pessoa
              edita o conteúdo.
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            {pending && !claim && p.can_edit && (
              <button
                type="button"
                className="btn min-h-11 px-4"
                disabled={busy !== undefined}
                onClick={() => run("claim", "claim", () => api(`${base}/claim`, { method: "POST" }), "Posse assumida. Você pode editar o conteúdo.")}
              >
                {busy === "claim" ? "Assumindo…" : "Assumir"}
              </button>
            )}
            {claim?.mine && (
              <button
                type="button"
                className="btn-ghost min-h-11 px-4"
                disabled={busy !== undefined || editorDirty}
                aria-describedby={editorDirty ? "release-hint" : undefined}
                onClick={() => run("claim", "release", () => api(`${base}/claim`, { method: "DELETE" }), "Posse solta. Outra pessoa pode assumir.")}
              >
                {busy === "release" ? "Soltando…" : "Soltar Posse"}
              </button>
            )}
            {/* Quem pode soltar a Posse alheia é a API que diz (can_release). */}
            {claim && !claim.mine && claim.can_release && !confirmRelease && (
              <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setConfirmRelease(true)}>
                Soltar a Posse de {claim.holder_name}
              </button>
            )}
          </div>
          {claim?.mine && editorDirty && (
            <p id="release-hint" className="text-sm text-stone-600">Salve ou desfaça as alterações antes de soltar a Posse.</p>
          )}
          {claim && !claim.mine && confirmRelease && (
            <div role="group" aria-label="Confirmar soltar a Posse" className="flex flex-wrap items-center gap-3 rounded-md border border-stone-300 bg-stone-50 p-3">
              <p className="flex-1 text-sm">
                {claim.holder_name} deixa de ter a Posse e pode perder o que não salvou. Fica registrado na Auditoria.
              </p>
              <button
                type="button"
                className="btn min-h-11 px-4"
                disabled={busy !== undefined}
                onClick={async () => {
                  await run("claim", "release", () => api(`${base}/claim`, { method: "DELETE" }), "Posse solta.");
                  setConfirmRelease(false);
                }}
              >
                {busy === "release" ? "Soltando…" : "Soltar Posse"}
              </button>
              <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setConfirmRelease(false)}>
                Cancelar
              </button>
            </div>
          )}
          <Notice feedback={feedback} area="claim" />
        </div>
      )}

      {/* Conteúdo da Versão em análise */}
      {pending && p.pending_content && (
        <div className="space-y-4 border-t border-stone-200 pt-6">
          <h3 className="font-semibold">
            Resumo da Versão {pending.seq} (em análise)
          </h3>
          {p.pending_content.authors.length > 0 && (
            <p className="text-sm text-stone-600">Autoria: {names(p.pending_content.authors, p.people)}</p>
          )}
          {p.can_edit ? (
            <ContentEditor
              key={pending.id}
              base={base}
              version={pending}
              content={p.pending_content}
              allTopics={p.topics}
              people={p.people}
              reload={reload}
              onDirtyChange={setEditorDirty}
            />
          ) : (
            <ContentView content={p.pending_content} topicsById={topicsById} />
          )}
        </div>
      )}

      {/* Aprovação */}
      {pending && (
        <div className="space-y-3 border-t border-stone-200 pt-6">
          <h3 className="font-semibold">Aprovar a Versão {pending.seq}</h3>
          {p.can_approve ? (
            <>
              {p.approval_breaks_four_eyes && (
                <p className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-950">
                  Você está aprovando o próprio trabalho; ficará Segunda revisão pendente. O Selo fica &ldquo;revisado por 1
                  pessoa&rdquo; até outra pessoa confirmar.
                </p>
              )}
              {editorDirty && (
                <p className="text-sm text-stone-700">Há alterações não salvas no editor. Salve ou desfaça antes de aprovar.</p>
              )}
              {!confirmApprove ? (
                <button type="button" className="btn-lg" disabled={busy !== undefined || editorDirty} onClick={() => setConfirmApprove(true)}>
                  Aprovar
                </button>
              ) : (
                <div role="group" aria-label="Confirmar aprovação" className="flex flex-wrap items-center gap-3 rounded-md border border-stone-300 bg-stone-50 p-3">
                  <p className="flex-1 text-sm">
                    Ao aprovar, o Resumo, o Resumo curto e os Temas desta Versão passam a aparecer no site, e a Posse é solta.
                  </p>
                  <button
                    type="button"
                    className="btn min-h-11 px-4"
                    disabled={busy !== undefined || editorDirty}
                    onClick={async () => {
                      await run(
                        "approve",
                        "approve",
                        // a revision do texto mostrado: se alguém gravou depois, a API
                        // recusa (stale_content) e a recarga mostra o texto novo
                        () =>
                          api(`${base}/versions/${encodeURIComponent(pending.id)}/approve`, {
                            method: "POST",
                            body: JSON.stringify({ revision: p.pending_content?.revision ?? 0 }),
                          }),
                        p.approval_breaks_four_eyes
                          ? "Aprovada. Já aparece no site, com Segunda revisão pendente."
                          : "Aprovada. Já aparece no site com o Selo “revisado por 2 pessoas”.",
                      );
                      setConfirmApprove(false);
                    }}
                  >
                    {busy === "approve" ? "Aprovando…" : p.approval_breaks_four_eyes ? "Aprovar mesmo assim" : "Aprovar e publicar"}
                  </button>
                  <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setConfirmApprove(false)}>
                    Cancelar
                  </button>
                </div>
              )}
            </>
          ) : p.approve_blockers.length > 0 ? (
            <div className="text-stone-700">
              <p>Ainda não dá para você aprovar:</p>
              <ul className="list-disc pl-5">
                {p.approve_blockers.map((b) => (
                  <li key={b.code}>{b.message}</li>
                ))}
              </ul>
            </div>
          ) : (
            <p className="text-stone-700">Você não pode aprovar esta Versão.</p>
          )}
          <Notice feedback={feedback} area="approve" />
        </div>
      )}

      {/* Conteúdo publicado */}
      {current && p.current_content && (
        <details className="border-t border-stone-200 pt-6">
          <summary className="btn-ghost min-h-11 cursor-pointer px-4">Ver o conteúdo publicado (Versão {current.seq}, vigente)</summary>
          <div className="mt-4 space-y-3">
            {p.current_content.authors.length > 0 && (
              <p className="text-sm text-stone-600">Autoria: {names(p.current_content.authors, p.people)}</p>
            )}
            <ContentView content={p.current_content} topicsById={topicsById} />
          </div>
        </details>
      )}
    </section>
  );
}

function ContentView({ content, topicsById }: { content: Content; topicsById: Map<string, Topic> }) {
  return (
    <dl className="space-y-4">
      <div>
        <dt className="label">Resumo</dt>
        <dd>
          <Paragraphs text={content.summary} empty="Sem Resumo ainda." />
        </dd>
      </div>
      <div>
        <dt className="label">Resumo curto</dt>
        <dd className="[overflow-wrap:anywhere]">{content.short_summary || <span className="text-stone-600">Sem Resumo curto ainda.</span>}</dd>
      </div>
      <div>
        <dt className="label">Temas</dt>
        <dd>
          {content.topic_ids.length === 0
            ? <span className="text-stone-600">Nenhum Tema escolhido.</span>
            : content.topic_ids.map((id) => {
                const t = topicsById.get(id);
                return t ? (t.active ? t.name : `${t.name} (desativado)`) : "Tema removido";
              }).join(", ")}
        </dd>
      </div>
      {content.needs_confirmation && (
        <p className="text-sm text-amber-900">Trazido da Versão anterior, ainda a confirmar.</p>
      )}
    </dl>
  );
}

type Draft = { summary: string; short: string; topicIds: string[] };

function fromContent(c: Content): Draft {
  return { summary: c.summary, short: c.short_summary, topicIds: [...c.topic_ids] };
}

function sameDraft(a: Draft, b: Draft) {
  return (
    a.summary === b.summary &&
    a.short === b.short &&
    a.topicIds.length === b.topicIds.length &&
    a.topicIds.every((id) => b.topicIds.includes(id))
  );
}

type FieldErrors = { summary?: string; short?: string; topics?: string; form?: string };

const FIELD_OF_CODE: Record<string, keyof FieldErrors> = {
  summary_too_long: "summary",
  short_summary_too_long: "short",
  too_many_topics: "topics",
  duplicate_topic: "topics",
  topic_inactive: "topics",
  topic_not_found: "topics",
};

function ContentEditor({
  base,
  version,
  content,
  allTopics,
  people,
  reload,
  onDirtyChange,
}: {
  base: string;
  version: Version;
  content: Content;
  allTopics: Topic[];
  people: Record<string, string>;
  reload: () => Promise<Detail | undefined>;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const { api } = useTeamSession();
  // saved: o que está gravado na API e sobre o qual o rascunho foi escrito.
  const [saved, setSaved] = useState<Content>(content);
  const [draft, setDraft] = useState<Draft>(() => fromContent(content));
  const [errors, setErrors] = useState<FieldErrors>({});
  const [stale, setStale] = useState(false);
  // Conteúdo gravado por outra pessoa enquanto havia texto não salvo aqui.
  const [theirs, setTheirs] = useState<Content>();
  const [busy, setBusy] = useState<"save" | "confirm" | "reload">();
  const [notice, setNotice] = useState<string>();
  const [seenRevision, setSeenRevision] = useState(content.revision);

  const dirty = !sameDraft(draft, fromContent(saved));

  // A API mandou um revision novo (recarga da página): sem texto pendente,
  // o rascunho acompanha; com texto pendente, guardamos o deles à parte e o
  // próximo salvar grava por cima, com o revision novo.
  // (O revision só sobe; um prop mais velho que o já visto é ignorado.)
  if (content.revision > seenRevision) {
    setSeenRevision(content.revision);
    setSaved(content);
    if (dirty) setTheirs(content);
    else setDraft(fromContent(content));
  }

  useEffect(() => {
    onDirtyChange(dirty);
  }, [dirty, onDirtyChange]);
  useEffect(() => () => onDirtyChange(false), [onDirtyChange]);

  const summaryCount = charCount(draft.summary);
  const shortCount = charCount(draft.short);
  const overLimit = summaryCount > SUMMARY_MAX || shortCount > SHORT_MAX;

  // Ativos para escolher, mais os desativados que já estavam escolhidos.
  const chosenBefore = new Set([...saved.topic_ids, ...draft.topicIds]);
  const offered = allTopics.filter((t) => t.active || chosenBefore.has(t.id));

  function handleError(e: unknown) {
    if (e instanceof ApiError && e.code === "stale_content") {
      setStale(true);
      setErrors({});
      return;
    }
    if (e instanceof ApiError && FIELD_OF_CODE[e.code]) {
      setErrors({ [FIELD_OF_CODE[e.code]]: e.message });
      return;
    }
    setErrors({ form: errMessage(e, "Não foi possível salvar. Tente de novo.") });
    if (e instanceof ApiError && (e.code === "claim_taken" || e.code === "version_not_editable")) void reload();
  }

  function accept(c: Content, message: string) {
    setSaved(c);
    setDraft(fromContent(c));
    setSeenRevision(c.revision);
    setTheirs(undefined);
    setStale(false);
    setErrors({});
    setNotice(message);
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy("save");
    setNotice(undefined);
    try {
      const c = await api<Content>(`${base}/versions/${encodeURIComponent(version.id)}/content`, {
        method: "PUT",
        body: JSON.stringify({ summary: draft.summary, short_summary: draft.short, topic_ids: draft.topicIds, revision: saved.revision }),
      });
      accept(c, "Conteúdo salvo.");
      await reload();
    } catch (err) {
      handleError(err);
    } finally {
      setBusy(undefined);
    }
  }

  async function confirmCarried() {
    setBusy("confirm");
    setNotice(undefined);
    try {
      const c = await api<Content>(`${base}/versions/${encodeURIComponent(version.id)}/content/confirm`, {
        method: "POST",
        body: JSON.stringify({ revision: saved.revision }),
      });
      accept(c, "Conteúdo trazido confirmado.");
      await reload();
    } catch (err) {
      handleError(err);
    } finally {
      setBusy(undefined);
    }
  }

  async function loadTheirs() {
    setBusy("reload");
    try {
      await reload();
      setStale(false);
    } finally {
      setBusy(undefined);
    }
  }

  function toggleTopic(id: string, on: boolean) {
    setDraft((d) => ({ ...d, topicIds: on ? [...d.topicIds, id] : d.topicIds.filter((x) => x !== id) }));
    setErrors((er) => ({ ...er, topics: undefined }));
  }

  return (
    <form onSubmit={save} className="space-y-5" aria-describedby={dirty ? "unsaved" : undefined}>
      {saved.needs_confirmation && (
        <div className="space-y-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-3 text-sm text-amber-950">
          <p>
            Este conteúdo veio da Versão anterior. Confira se continua valendo para o texto novo e confirme. Salvar não
            confirma.
          </p>
          <button type="button" className="btn min-h-11 px-4" disabled={busy !== undefined || dirty} onClick={confirmCarried}>
            {busy === "confirm" ? "Confirmando…" : "Confirmar conteúdo trazido"}
          </button>
          {dirty && <p>Salve as alterações antes de confirmar.</p>}
        </div>
      )}

      {stale && (
        <div role="alert" className="space-y-2 rounded-md border border-red-300 bg-red-50 px-3 py-3 text-sm text-red-900">
          <p>
            Outra pessoa salvou este conteúdo depois que você abriu a página. Nada do seu texto foi gravado, e ele continua nos
            campos abaixo.
          </p>
          <button type="button" className="btn-ghost min-h-11 px-4" disabled={busy !== undefined} onClick={loadTheirs}>
            {busy === "reload" ? "Carregando…" : "Carregar o texto salvo para comparar"}
          </button>
        </div>
      )}

      {theirs && (
        <div className="space-y-3 rounded-md border border-stone-300 bg-stone-50 p-3">
          <p className="text-sm font-medium">
            Texto salvo{theirs.updated_by ? ` por ${people[theirs.updated_by] ?? "outra pessoa"}` : ""} em{" "}
            {fmtDateTime(theirs.updated_at)}. Se você salvar, o seu texto substitui este.
          </p>
          <div className="text-sm">
            <p className="label">Resumo salvo</p>
            <Paragraphs text={theirs.summary} empty="Vazio." />
            <p className="label mt-3">Resumo curto salvo</p>
            <p className="[overflow-wrap:anywhere]">{theirs.short_summary || "Vazio."}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => accept(theirs, "Você está com o texto salvo; o seu foi descartado.")}>
              Usar o texto salvo (descarta o seu)
            </button>
            <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setTheirs(undefined)}>
              Manter o meu
            </button>
          </div>
        </div>
      )}

      <FormError message={errors.form} />

      <div>
        <label htmlFor="summary" className="label">Resumo</label>
        <p id="summary-hint" className="mb-1 text-sm text-stone-600">
          Texto simples, em linguagem direta. Deixe uma linha em branco entre parágrafos.
        </p>
        <textarea
          id="summary"
          value={draft.summary}
          onChange={(e) => {
            setDraft((d) => ({ ...d, summary: e.target.value }));
            setErrors((er) => ({ ...er, summary: undefined }));
          }}
          rows={12}
          aria-invalid={errors.summary || summaryCount > SUMMARY_MAX ? true : undefined}
          aria-describedby="summary-hint summary-count summary-error"
          className="input min-h-40 text-base leading-relaxed"
        />
        <p id="summary-count" className={`mt-1 text-sm ${summaryCount > SUMMARY_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
          {summaryCount.toLocaleString("pt-BR")} de {SUMMARY_MAX.toLocaleString("pt-BR")} caracteres
          {summaryCount > SUMMARY_MAX && `: passou ${(summaryCount - SUMMARY_MAX).toLocaleString("pt-BR")}. Encurte para salvar.`}
        </p>
        <FieldError id="summary-error" message={errors.summary} />
      </div>

      <div>
        <label htmlFor="short" className="label">Resumo curto</label>
        <p id="short-hint" className="mb-1 text-sm text-stone-600">
          Uma ou duas frases para a lista e para o compartilhamento. Nada é cortado: se passar do limite, encurte.
        </p>
        <textarea
          id="short"
          value={draft.short}
          onChange={(e) => {
            setDraft((d) => ({ ...d, short: e.target.value }));
            setErrors((er) => ({ ...er, short: undefined }));
          }}
          rows={3}
          aria-invalid={errors.short || shortCount > SHORT_MAX ? true : undefined}
          aria-describedby="short-hint short-count short-error"
          className="input text-base leading-relaxed"
        />
        <p id="short-count" className={`mt-1 text-sm ${shortCount > SHORT_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
          {shortCount} de {SHORT_MAX} caracteres
          {shortCount > SHORT_MAX && `: passou ${shortCount - SHORT_MAX}. Encurte para salvar.`}
        </p>
        <FieldError id="short-error" message={errors.short} />
      </div>

      <fieldset aria-describedby="topics-count topics-error">
        <legend className="label">Temas (até {TOPICS_MAX})</legend>
        {offered.length === 0 ? (
          <p className="text-sm text-stone-600">Nenhum Tema cadastrado ainda. Quem cria os Temas é o Admin.</p>
        ) : (
          <ul className="grid gap-1 sm:grid-cols-2">
            {offered.map((t) => {
              const checked = draft.topicIds.includes(t.id);
              const disabled = !checked && (!t.active || draft.topicIds.length >= TOPICS_MAX);
              return (
                <li key={t.id}>
                  <label className={`flex min-h-11 items-center gap-3 rounded-md px-2 ${disabled ? "text-stone-500" : "cursor-pointer hover:bg-stone-50"}`}>
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={disabled}
                      onChange={(e) => toggleTopic(t.id, e.target.checked)}
                      className="size-5 accent-stone-900"
                    />
                    <span className="[overflow-wrap:anywhere]">
                      {t.name}
                      {!t.active && " (desativado: se tirar, não volta)"}
                    </span>
                  </label>
                </li>
              );
            })}
          </ul>
        )}
        <p id="topics-count" className="mt-1 text-sm text-stone-600">
          {draft.topicIds.length} de {TOPICS_MAX} escolhidos
        </p>
        <FieldError id="topics-error" message={errors.topics} />
      </fieldset>

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" className="btn-lg" disabled={busy !== undefined || !dirty || overLimit}>
          {busy === "save" ? "Salvando…" : "Salvar"}
        </button>
        {dirty && (
          <button type="button" className="btn-ghost min-h-12 px-4" disabled={busy !== undefined} onClick={() => setDraft(fromContent(saved))}>
            Desfazer alterações
          </button>
        )}
        <p id="unsaved" role="status" className="text-sm text-stone-600">
          {dirty ? "Alterações não salvas." : (notice ?? `Salvo em ${fmtDateTime(saved.updated_at)}.`)}
        </p>
      </div>
    </form>
  );
}

function NewVersionForm({ base, onCancel, onDone }: { base: string; onCancel: () => void; onDone: () => Promise<void> }) {
  const { api } = useTeamSession();
  const [label, setLabel] = useState("");
  const [url, setUrl] = useState("");
  const [errors, setErrors] = useState<{ label?: string; url?: string; form?: string }>({});
  const [pending, setPending] = useState(false);
  const labelCount = charCount(label);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setPending(true);
    setErrors({});
    try {
      await api(`${base}/versions`, { method: "POST", body: JSON.stringify({ label, official_text_url: url }) });
      await onDone();
    } catch (err) {
      const message = errMessage(err, "Não foi possível registrar a Versão.");
      const code = err instanceof ApiError ? err.code : "";
      if (code === "invalid_label") setErrors({ label: message });
      else if (code === "invalid_official_text_url") setErrors({ url: message });
      else setErrors({ form: message });
    } finally {
      setPending(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-4 rounded-md border border-stone-300 bg-stone-50 p-3">
      <p className="text-sm text-stone-700">
        Use quando a Casa publicar um texto novo (um substitutivo, por exemplo). O conteúdo da Versão vigente é trazido para a
        nova, marcado a confirmar, e a Proposição volta para revisão.
      </p>
      <FormError message={errors.form} />
      <div>
        <label htmlFor="version-label" className="label">Rótulo</label>
        <input
          id="version-label"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          required
          placeholder="Substitutivo da CCJ"
          autoComplete="off"
          aria-invalid={errors.label || labelCount > LABEL_MAX ? true : undefined}
          aria-describedby="version-label-count version-label-error"
          className="input min-h-11 text-base"
        />
        <p id="version-label-count" className={`mt-1 text-sm ${labelCount > LABEL_MAX ? "text-red-800" : "text-stone-600"}`}>
          {labelCount} de {LABEL_MAX} caracteres
        </p>
        <FieldError id="version-label-error" message={errors.label} />
      </div>
      <div>
        <label htmlFor="version-url" className="label">Link oficial do Documento integral</label>
        <input
          id="version-url"
          type="url"
          inputMode="url"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          required
          placeholder="https://"
          autoComplete="off"
          aria-invalid={errors.url ? true : undefined}
          aria-describedby="version-url-error"
          className="input min-h-11 text-base"
        />
        <FieldError id="version-url-error" message={errors.url} />
      </div>
      <div className="flex flex-wrap gap-2">
        <button type="submit" className="btn min-h-11 px-4" disabled={pending || labelCount === 0 || labelCount > LABEL_MAX}>
          {pending ? "Registrando…" : "Registrar Versão"}
        </button>
        <button type="button" className="btn-ghost min-h-11 px-4" onClick={onCancel}>
          Cancelar
        </button>
      </div>
    </form>
  );
}
