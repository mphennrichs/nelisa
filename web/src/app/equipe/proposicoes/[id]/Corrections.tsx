"use client";

// Correção menor e Errata do conteúdo publicado (Versão vigente). Quem pode
// corrigir, conferir, aprovar, recusar ou retirar vem da API (can_*); aqui
// só mostramos e chamamos. Toda confirmação é na própria linha, sem
// window.confirm.
import { useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FieldError, FormError } from "@/lib/team/ui";
import { CONTENT_FIELD_LABEL, ERRATUM_STATE_LABEL, charCount, fmtDate, fmtDateTime, paragraphs } from "@/lib/labels";

type Detail = components["schemas"]["TeamPropositionDetail"];
type Version = components["schemas"]["PropositionVersion"];
type Content = components["schemas"]["VersionContent"];
type Topic = components["schemas"]["Topic"];
type Snapshot = components["schemas"]["ContentSnapshot"];
type MinorFix = components["schemas"]["TeamMinorFix"];
type Erratum = components["schemas"]["TeamErratum"];

// Limites do contrato (openapi: UpdateContentRequest, ProposeErratumRequest).
const SUMMARY_MAX = 5000;
const SHORT_MAX = 280;
const TOPICS_MAX = 3;
const NOTE_MAX = 500;
const REASON_MAX = 1000;

type Feedback = { kind: "ok" | "error"; message: string };

function errMessage(e: unknown, fallback: string) {
  return e instanceof ApiError ? e.message : fallback;
}

function Paragraphs({ text }: { text: string }) {
  const ps = paragraphs(text);
  if (ps.length === 0) return <p className="text-stone-600">Vazio.</p>;
  return (
    <div className="space-y-2">
      {ps.map((p, i) => (
        <p key={i} className="whitespace-pre-line leading-relaxed [overflow-wrap:anywhere]">{p}</p>
      ))}
    </div>
  );
}

function topicNames(ids: string[], topicsById: Map<string, Topic>) {
  if (ids.length === 0) return "Nenhum Tema";
  return ids.map((id) => topicsById.get(id)?.name ?? "Tema removido").join(", ");
}

function who(id: string | null | undefined, people: Record<string, string>) {
  return id ? (people[id] ?? "pessoa removida") : "—";
}

/** Um campo do conteúdo, antes e depois, lado a lado (empilhado no celular). */
function BeforeAfter({ label, before, after }: { label: string; before: React.ReactNode; after: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <p className="font-medium">{label}</p>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="rounded-md border border-stone-200 bg-stone-50 p-3 text-sm">
          <p className="label">Antes</p>
          {before}
        </div>
        <div className="rounded-md border border-stone-300 bg-white p-3 text-sm">
          <p className="label">Depois</p>
          {after}
        </div>
      </div>
    </div>
  );
}

function SnapshotDiff({ before, after, topicsById }: { before: Snapshot; after: Snapshot; topicsById: Map<string, Topic> }) {
  const parts: React.ReactNode[] = [];
  if (before.summary !== after.summary) {
    parts.push(<BeforeAfter key="s" label="Resumo" before={<Paragraphs text={before.summary} />} after={<Paragraphs text={after.summary} />} />);
  }
  if (before.short_summary !== after.short_summary) {
    parts.push(
      <BeforeAfter
        key="c"
        label="Resumo curto"
        before={<p className="[overflow-wrap:anywhere]">{before.short_summary || "Vazio."}</p>}
        after={<p className="[overflow-wrap:anywhere]">{after.short_summary || "Vazio."}</p>}
      />,
    );
  }
  if (before.topic_ids.join() !== after.topic_ids.join()) {
    parts.push(
      <BeforeAfter
        key="t"
        label="Temas"
        before={<p>{topicNames(before.topic_ids, topicsById)}</p>}
        after={<p>{topicNames(after.topic_ids, topicsById)}</p>}
      />,
    );
  }
  if (parts.length === 0) return <p className="text-sm text-stone-600">Nenhuma diferença no texto.</p>;
  return <div className="space-y-4">{parts}</div>;
}

/** Confirmação na linha: o botão vira uma caixa com a consequência, confirmar e cancelar. */
function InlineConfirm({
  label,
  question,
  confirmLabel,
  busyLabel,
  busy,
  disabled,
  primary = false,
  onConfirm,
}: {
  label: string;
  question: string;
  confirmLabel: string;
  busyLabel: string;
  busy: boolean;
  disabled: boolean;
  primary?: boolean;
  onConfirm: () => Promise<boolean>;
}) {
  const [open, setOpen] = useState(false);
  if (!open) {
    return (
      <button type="button" className={primary ? "btn-lg" : "btn-ghost min-h-11 px-4"} disabled={disabled} onClick={() => setOpen(true)}>
        {label}
      </button>
    );
  }
  return (
    <div role="group" aria-label={label} className="flex w-full flex-wrap items-center gap-3 rounded-md border border-stone-300 bg-stone-50 p-3">
      <p className="min-w-[12rem] flex-1 text-sm">{question}</p>
      <button
        type="button"
        className="btn min-h-11 px-4"
        disabled={disabled}
        onClick={async () => {
          if (await onConfirm()) setOpen(false);
        }}
      >
        {busy ? busyLabel : confirmLabel}
      </button>
      <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setOpen(false)}>
        Cancelar
      </button>
    </div>
  );
}

export default function Corrections({
  p,
  current,
  content,
  reload,
}: {
  p: Detail;
  current: Version;
  content: Content;
  reload: () => Promise<Detail | undefined>;
}) {
  const { api } = useTeamSession();
  const [feedback, setFeedback] = useState<Feedback>();
  const [busy, setBusy] = useState<string>();
  const [editor, setEditor] = useState<"minor" | "erratum">();
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");
  const [reasonError, setReasonError] = useState<string>();

  const base = `/api/team/propositions/${encodeURIComponent(p.id)}`;
  const topicsById = new Map(p.topics.map((t) => [t.id, t]));
  const pendingErratum = p.pending_erratum;
  const published: Snapshot = { summary: content.summary, short_summary: content.short_summary, topic_ids: content.topic_ids };

  async function run(key: string, fn: () => Promise<unknown>, done: string) {
    setBusy(key);
    setFeedback(undefined);
    try {
      await fn();
      setFeedback({ kind: "ok", message: done });
      await reload();
      return true;
    } catch (e) {
      setFeedback({ kind: "error", message: errMessage(e, "A ação falhou. Tente de novo.") });
      await reload();
      return false;
    } finally {
      setBusy(undefined);
    }
  }

  const erratumBase = (e: Erratum) => `${base}/versions/${encodeURIComponent(e.version_id)}/errata/${encodeURIComponent(e.id)}`;
  const claimHolder = p.claim && !p.claim.mine ? p.claim.holder_name : undefined;

  return (
    <div className="space-y-6 border-t border-stone-200 pt-6">
      <div className="space-y-2">
        <h3 className="font-semibold">Corrigir o conteúdo publicado (Versão {current.seq})</h3>
        <p className="text-sm text-stone-700">
          <strong>Correção menor</strong> não muda o sentido (digitação, formatação, link): entra na hora, sem aviso ao leitor, e
          fica para um Revisor conferir. <strong>Errata</strong> muda o sentido: só entra quando outro Revisor aprovar, e aparece
          no site como nota datada (&ldquo;dizia X; o correto é Y&rdquo;).
        </p>
      </div>

      {feedback &&
        (feedback.kind === "error" ? (
          <FormError message={feedback.message} />
        ) : (
          <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">
            {feedback.message}
          </p>
        ))}

      {!editor && (p.can_minor_fix || p.can_propose_erratum) && (
        <div className="flex flex-wrap gap-2">
          {p.can_minor_fix && (
            <button type="button" className="btn min-h-11 px-4" onClick={() => setEditor("minor")}>
              Corrigir (correção menor)
            </button>
          )}
          {p.can_propose_erratum && (
            <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setEditor("erratum")}>
              Propor Errata
            </button>
          )}
        </div>
      )}
      {!editor && !p.can_minor_fix && p.can_propose_erratum && claimHolder && (
        <p className="text-sm text-stone-700">
          A Correção menor exige a Posse, que está com {claimHolder}. A Errata não exige.
        </p>
      )}
      {!editor && !p.can_minor_fix && !p.can_propose_erratum && pendingErratum && (
        <p className="text-sm text-stone-700">Há uma Errata pendente: decida-a antes de corrigir o texto de novo.</p>
      )}

      {editor && (
        <CorrectionForm
          key={editor}
          mode={editor}
          base={base}
          version={current}
          content={content}
          allTopics={p.topics}
          onCancel={() => setEditor(undefined)}
          onDone={async (message) => {
            setEditor(undefined);
            setFeedback({ kind: "ok", message });
            await reload();
          }}
          reload={reload}
        />
      )}

      {/* Errata pendente */}
      {pendingErratum && (
        <div className="space-y-4 rounded-md border border-amber-300 bg-amber-50/40 p-4">
          <h4 className="font-semibold">Errata pendente</h4>
          <p className="text-sm text-stone-700">
            Proposta por {who(pendingErratum.created_by, p.people)} em {fmtDateTime(pendingErratum.created_at)}. Enquanto não for
            aprovada, o site mostra o texto antigo.
          </p>
          <div className="rounded-md border border-stone-300 bg-white p-3">
            <p className="label">Nota que o leitor vai ver</p>
            <p className="[overflow-wrap:anywhere]">
              Dizia: {pendingErratum.said}
              <br />O correto é: {pendingErratum.correct}
            </p>
          </div>
          <SnapshotDiff before={published} after={pendingErratum.proposed} topicsById={topicsById} />
          {pendingErratum.stale && (
            <p role="alert" className="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-900">
              O conteúdo publicado mudou depois que esta Errata foi proposta. Ela não pode mais ser aprovada: retire-a ou recuse-a e
              proponha de novo sobre o texto atual.
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            {pendingErratum.can_decide_erratum && !pendingErratum.stale && !rejecting && (
              <InlineConfirm
                label="Aprovar Errata"
                question="Ao aprovar, o conteúdo publicado passa a ser o proposto e a nota aparece no site com a data de hoje."
                confirmLabel="Aprovar e publicar"
                busyLabel="Aprovando…"
                busy={busy === "approve"}
                disabled={busy !== undefined}
                primary
                onConfirm={() =>
                  run(
                    "approve",
                    () =>
                      api(`${erratumBase(pendingErratum)}/approve`, {
                        method: "POST",
                        // a revision do texto publicado que está nesta tela
                        body: JSON.stringify({ revision: content.revision }),
                      }),
                    "Errata aprovada. O site já mostra o texto corrigido e a nota.",
                  )
                }
              />
            )}
            {pendingErratum.can_decide_erratum && !rejecting && (
              <button type="button" className="btn-ghost min-h-11 px-4" disabled={busy !== undefined} onClick={() => setRejecting(true)}>
                Recusar Errata
              </button>
            )}
            {pendingErratum.can_withdraw_erratum && !rejecting && (
              <InlineConfirm
                label="Retirar Errata"
                question="A Errata deixa de valer e nada muda no site."
                confirmLabel="Retirar"
                busyLabel="Retirando…"
                busy={busy === "withdraw"}
                disabled={busy !== undefined}
                onConfirm={() =>
                  run("withdraw", () => api(`${erratumBase(pendingErratum)}/withdraw`, { method: "POST" }), "Errata retirada. Nada mudou no site.")
                }
              />
            )}
          </div>
          {pendingErratum.can_decide_erratum && rejecting && (
            <form
              className="space-y-3 rounded-md border border-stone-300 bg-stone-50 p-3"
              onSubmit={async (e) => {
                e.preventDefault();
                if (charCount(reason) === 0) {
                  setReasonError("Escreva o motivo da recusa.");
                  return;
                }
                const ok = await run(
                  "reject",
                  () => api(`${erratumBase(pendingErratum)}/reject`, { method: "POST", body: JSON.stringify({ reason }) }),
                  "Errata recusada. Nada mudou no site.",
                );
                if (ok) {
                  setRejecting(false);
                  setReason("");
                }
              }}
            >
              <div>
                <label htmlFor="reject-reason" className="label">Motivo da recusa (só a equipe vê)</label>
                <textarea
                  id="reject-reason"
                  value={reason}
                  onChange={(e) => {
                    setReason(e.target.value);
                    setReasonError(undefined);
                  }}
                  rows={3}
                  aria-invalid={reasonError || charCount(reason) > REASON_MAX ? true : undefined}
                  aria-describedby="reject-reason-count reject-reason-error"
                  className="input text-base leading-relaxed"
                />
                <p id="reject-reason-count" className={`mt-1 text-sm ${charCount(reason) > REASON_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
                  {charCount(reason)} de {REASON_MAX.toLocaleString("pt-BR")} caracteres
                </p>
                <FieldError id="reject-reason-error" message={reasonError} />
              </div>
              <div className="flex flex-wrap gap-2">
                <button type="submit" className="btn min-h-11 px-4" disabled={busy !== undefined || charCount(reason) > REASON_MAX}>
                  {busy === "reject" ? "Recusando…" : "Recusar"}
                </button>
                <button
                  type="button"
                  className="btn-ghost min-h-11 px-4"
                  onClick={() => {
                    setRejecting(false);
                    setReasonError(undefined);
                  }}
                >
                  Cancelar
                </button>
              </div>
            </form>
          )}
          {!pendingErratum.can_decide_erratum && !pendingErratum.can_withdraw_erratum && (
            <p className="text-sm text-stone-700">Outro Revisor (que não propôs) aprova ou recusa.</p>
          )}
        </div>
      )}

      {/* Correções a conferir */}
      {p.minor_fixes.length > 0 && (
        <div className="space-y-3">
          <h4 className="font-semibold">Correções a conferir</h4>
          <ul className="space-y-4">
            {p.minor_fixes.map((f: MinorFix) => (
              <li key={f.id} className="space-y-3 rounded-md border border-stone-200 p-3">
                <p className="text-sm text-stone-700">
                  {who(f.created_by, p.people)} corrigiu {f.fields.map((x) => CONTENT_FIELD_LABEL[x]).join(", ")} em{" "}
                  {fmtDateTime(f.created_at)}
                  {f.version_id !== current.id && " (numa Versão que já não é a vigente)"}.
                </p>
                <SnapshotDiff before={f.before} after={f.after} topicsById={topicsById} />
                {f.can_check_minor_fix ? (
                  <InlineConfirm
                    label="Conferir"
                    question="Conferir é dizer que a correção não muda o sentido. Se discordar, faça outra Correção menor ou proponha uma Errata."
                    confirmLabel="Confirmar conferência"
                    busyLabel="Conferindo…"
                    busy={busy === `check-${f.id}`}
                    disabled={busy !== undefined}
                    onConfirm={() =>
                      run(
                        `check-${f.id}`,
                        () => api(`${base}/minor-fixes/${encodeURIComponent(f.id)}/check`, { method: "POST" }),
                        "Correção conferida. Saiu da Fila.",
                      )
                    }
                  />
                ) : (
                  <p className="text-sm text-stone-600">Um Revisor ou Admin que não fez a correção confere.</p>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Histórico */}
      {p.errata.length > 0 && (
        <details>
          <summary className="btn-ghost min-h-11 cursor-pointer px-4">Histórico de Erratas ({p.errata.length})</summary>
          <ul className="mt-3 space-y-3">
            {p.errata.map((e) => (
              <li key={e.id} className="space-y-1 rounded-md border border-stone-200 p-3 text-sm">
                <p className="font-medium">
                  {ERRATUM_STATE_LABEL[e.state]}
                  {e.decided_at ? ` em ${fmtDate(e.decided_at)}` : ""}
                  {e.state === "approved" || e.state === "rejected" ? ` por ${who(e.decided_by, p.people)}` : ""}
                </p>
                <p className="text-stone-700">
                  Proposta por {who(e.created_by, p.people)} em {fmtDate(e.created_at)}.
                </p>
                <p className="[overflow-wrap:anywhere]">
                  Dizia: {e.said} · O correto é: {e.correct}
                </p>
                {e.state === "rejected" && e.reason && <p className="text-stone-700 [overflow-wrap:anywhere]">Motivo: {e.reason}</p>}
                {e.state === "pending" && e.can_withdraw_erratum && (
                  <button
                    type="button"
                    className="btn-ghost min-h-11 px-4"
                    disabled={busy !== undefined}
                    onClick={() => run(`withdraw-${e.id}`, () => api(`${erratumBase(e)}/withdraw`, { method: "POST" }), "Errata retirada.")}
                  >
                    {busy === `withdraw-${e.id}` ? "Retirando…" : "Retirar (a Versão já não é a vigente)"}
                  </button>
                )}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}

type FieldErrors = { summary?: string; short?: string; topics?: string; said?: string; correct?: string; form?: string };

const FIELD_OF_CODE: Record<string, keyof FieldErrors> = {
  summary_too_long: "summary",
  short_summary_too_long: "short",
  too_many_topics: "topics",
  duplicate_topic: "topics",
  topic_inactive: "topics",
  topic_not_found: "topics",
};

// Erros que mudam o que a tela deve mostrar: recarregar o detalhe.
const RELOAD_CODES = new Set(["claim_taken", "erratum_pending", "not_ready", "version_not_current"]);

function CorrectionForm({
  mode,
  base,
  version,
  content,
  allTopics,
  onCancel,
  onDone,
  reload,
}: {
  mode: "minor" | "erratum";
  base: string;
  version: Version;
  content: Content;
  allTopics: Topic[];
  onCancel: () => void;
  onDone: (message: string) => Promise<void>;
  reload: () => Promise<Detail | undefined>;
}) {
  const { api } = useTeamSession();
  // A revision do texto sobre o qual a correção foi escrita.
  const [baseRevision, setBaseRevision] = useState(content.revision);
  const [summary, setSummary] = useState(content.summary);
  const [short, setShort] = useState(content.short_summary);
  const [topicIds, setTopicIds] = useState<string[]>([...content.topic_ids]);
  const [said, setSaid] = useState("");
  const [correct, setCorrect] = useState("");
  const [errors, setErrors] = useState<FieldErrors>({});
  const [stale, setStale] = useState(false);
  const [pending, setPending] = useState(false);

  const erratum = mode === "erratum";
  const summaryCount = charCount(summary);
  const shortCount = charCount(short);
  const saidCount = charCount(said);
  const correctCount = charCount(correct);
  const overLimit =
    summaryCount > SUMMARY_MAX || shortCount > SHORT_MAX || (erratum && (saidCount > NOTE_MAX || correctCount > NOTE_MAX));
  const changed =
    summary !== content.summary || short !== content.short_summary || topicIds.join() !== content.topic_ids.join();
  const chosenBefore = new Set([...content.topic_ids, ...topicIds]);
  const offered = allTopics.filter((t) => t.active || chosenBefore.has(t.id));
  const id = (name: string) => `${mode}-${name}`;

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (erratum && (saidCount === 0 || correctCount === 0)) {
      setErrors({
        said: saidCount === 0 ? "Escreva o que o texto dizia." : undefined,
        correct: correctCount === 0 ? "Escreva o que é o correto." : undefined,
      });
      return;
    }
    setPending(true);
    setErrors({});
    const body = { summary, short_summary: short, topic_ids: topicIds, revision: baseRevision };
    try {
      if (erratum) {
        await api(`${base}/versions/${encodeURIComponent(version.id)}/errata`, {
          method: "POST",
          body: JSON.stringify({ ...body, said, correct }),
        });
        await onDone("Errata proposta. Nada muda no site até outro Revisor aprovar.");
      } else {
        await api(`${base}/versions/${encodeURIComponent(version.id)}/minor-fix`, { method: "PUT", body: JSON.stringify(body) });
        await onDone("Correção aplicada: o site já mostra o texto corrigido. Ela fica na Fila para um Revisor conferir.");
      }
    } catch (err) {
      const code = err instanceof ApiError ? err.code : "";
      const message = errMessage(err, "Não foi possível enviar. Tente de novo.");
      if (code === "stale_content") setStale(true);
      else if (FIELD_OF_CODE[code]) setErrors({ [FIELD_OF_CODE[code]]: message });
      else if (code === "invalid_erratum_note") setErrors(saidCount > NOTE_MAX || saidCount === 0 ? { said: message } : { correct: message });
      else setErrors({ form: message });
      if (RELOAD_CODES.has(code)) void reload();
    } finally {
      setPending(false);
    }
  }

  async function restart() {
    // Recomeça do texto publicado agora, como a API o devolve.
    const c = (await reload())?.current_content;
    if (!c) return;
    setBaseRevision(c.revision);
    setSummary(c.summary);
    setShort(c.short_summary);
    setTopicIds([...c.topic_ids]);
    setStale(false);
  }

  return (
    <form onSubmit={submit} className="space-y-5 rounded-md border border-stone-300 bg-stone-50 p-4">
      <h4 className="font-semibold">{erratum ? "Propor Errata" : "Correção menor"}</h4>
      <p className="text-sm text-stone-700">
        {erratum
          ? "Escreva o conteúdo como deve ficar e a nota que o leitor vai ver. Nada muda no site até outro Revisor aprovar."
          : "Só para o que não muda o sentido. Ao salvar, o site passa a mostrar o texto corrigido, sem aviso."}
      </p>

      {stale && (
        <div role="alert" className="space-y-2 rounded-md border border-red-300 bg-red-50 px-3 py-3 text-sm text-red-900">
          <p>O conteúdo publicado mudou depois que você abriu este editor. Nada foi enviado.</p>
          <button
            type="button"
            className="btn-ghost min-h-11 px-4"
            onClick={restart}
          >
            Recomeçar do texto atual
          </button>
        </div>
      )}
      <FormError message={errors.form} />

      <div>
        <label htmlFor={id("summary")} className="label">Resumo</label>
        <textarea
          id={id("summary")}
          value={summary}
          onChange={(e) => {
            setSummary(e.target.value);
            setErrors((er) => ({ ...er, summary: undefined }));
          }}
          rows={10}
          aria-invalid={errors.summary || summaryCount > SUMMARY_MAX ? true : undefined}
          aria-describedby={`${id("summary-count")} ${id("summary-error")}`}
          className="input min-h-40 text-base leading-relaxed"
        />
        <p id={id("summary-count")} className={`mt-1 text-sm ${summaryCount > SUMMARY_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
          {summaryCount.toLocaleString("pt-BR")} de {SUMMARY_MAX.toLocaleString("pt-BR")} caracteres
        </p>
        <FieldError id={id("summary-error")} message={errors.summary} />
      </div>

      <div>
        <label htmlFor={id("short")} className="label">Resumo curto</label>
        <textarea
          id={id("short")}
          value={short}
          onChange={(e) => {
            setShort(e.target.value);
            setErrors((er) => ({ ...er, short: undefined }));
          }}
          rows={3}
          aria-invalid={errors.short || shortCount > SHORT_MAX ? true : undefined}
          aria-describedby={`${id("short-count")} ${id("short-error")}`}
          className="input text-base leading-relaxed"
        />
        <p id={id("short-count")} className={`mt-1 text-sm ${shortCount > SHORT_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
          {shortCount} de {SHORT_MAX} caracteres
        </p>
        <FieldError id={id("short-error")} message={errors.short} />
      </div>

      <fieldset aria-describedby={`${id("topics-count")} ${id("topics-error")}`}>
        <legend className="label">Temas (de 1 a {TOPICS_MAX})</legend>
        <ul className="grid gap-1 sm:grid-cols-2">
          {offered.map((t) => {
            const checked = topicIds.includes(t.id);
            const disabled = !checked && (!t.active || topicIds.length >= TOPICS_MAX);
            return (
              <li key={t.id}>
                <label className={`flex min-h-11 items-center gap-3 rounded-md px-2 ${disabled ? "text-stone-500" : "cursor-pointer hover:bg-white"}`}>
                  <input
                    type="checkbox"
                    checked={checked}
                    disabled={disabled}
                    onChange={(e) => {
                      const on = e.target.checked;
                      setTopicIds((ids) => (on ? [...ids, t.id] : ids.filter((x) => x !== t.id)));
                      setErrors((er) => ({ ...er, topics: undefined }));
                    }}
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
        <p id={id("topics-count")} className="mt-1 text-sm text-stone-600">
          {topicIds.length} de {TOPICS_MAX} escolhidos
        </p>
        <FieldError id={id("topics-error")} message={errors.topics} />
      </fieldset>

      {erratum && (
        <div className="space-y-4 rounded-md border border-stone-300 bg-white p-3">
          <p className="text-sm text-stone-700">
            A nota sai no site assim: &ldquo;Em {fmtDate(new Date().toISOString())}: dizia …; o correto é …&rdquo;.
          </p>
          <div>
            <label htmlFor={id("said")} className="label">Dizia</label>
            <textarea
              id={id("said")}
              value={said}
              onChange={(e) => {
                setSaid(e.target.value);
                setErrors((er) => ({ ...er, said: undefined }));
              }}
              rows={2}
              aria-invalid={errors.said || saidCount > NOTE_MAX ? true : undefined}
              aria-describedby={`${id("said-count")} ${id("said-error")}`}
              className="input text-base leading-relaxed"
            />
            <p id={id("said-count")} className={`mt-1 text-sm ${saidCount > NOTE_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
              {saidCount} de {NOTE_MAX} caracteres
            </p>
            <FieldError id={id("said-error")} message={errors.said} />
          </div>
          <div>
            <label htmlFor={id("correct")} className="label">O correto é</label>
            <textarea
              id={id("correct")}
              value={correct}
              onChange={(e) => {
                setCorrect(e.target.value);
                setErrors((er) => ({ ...er, correct: undefined }));
              }}
              rows={2}
              aria-invalid={errors.correct || correctCount > NOTE_MAX ? true : undefined}
              aria-describedby={`${id("correct-count")} ${id("correct-error")}`}
              className="input text-base leading-relaxed"
            />
            <p id={id("correct-count")} className={`mt-1 text-sm ${correctCount > NOTE_MAX ? "font-medium text-red-800" : "text-stone-600"}`}>
              {correctCount} de {NOTE_MAX} caracteres
            </p>
            <FieldError id={id("correct-error")} message={errors.correct} />
          </div>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3">
        <button type="submit" className="btn-lg" disabled={pending || stale || overLimit || !changed}>
          {pending ? "Enviando…" : erratum ? "Propor Errata" : "Aplicar correção"}
        </button>
        <button type="button" className="btn-ghost min-h-12 px-4" disabled={pending} onClick={onCancel}>
          Cancelar
        </button>
        {!changed && <p className="text-sm text-stone-600">Mude o texto para poder enviar.</p>}
      </div>
    </form>
  );
}
