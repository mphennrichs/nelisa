"use client";

// Seção "Texto da Versão" da Proposição na equipe: a Estruturação (o texto
// oficial quebrado em Dispositivos). O Editor confere a árvore, corrige o
// texto e manda refazer, ou cola o texto quando a busca automática falhou.
// Toda permissão vem da API (can_edit_structuring, can_retry_structuring).
import { useEffect, useRef, useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError } from "@/lib/team/ui";
import { charCount, fmtDateTime } from "@/lib/labels";

type Detail = components["schemas"]["TeamPropositionDetail"];
type Version = components["schemas"]["PropositionVersion"];
type St = components["schemas"]["TeamStructuring"];
type Provision = components["schemas"]["TeamProvision"];
type Warning = components["schemas"]["StructuringWarning"];

// Limite do contrato (openapi: ReprocessStructuringRequest).
const TEXT_MAX = 500_000;
// Enquanto o Nelisa busca o PDF, a tela confere de tempos em tempos.
const POLL_MS = 5000;
const POLL_MAX = 36; // 3 minutos

const FAILURE_TEXT: Record<NonNullable<St["failure"]>, string> = {
  no_url: "A fonte oficial não informou o link do texto desta Versão.",
  host_not_allowed: "O link não é da Câmara nem do Senado: o Nelisa só busca sozinho nessas fontes.",
  not_found: "O link do texto oficial não abriu (página não encontrada).",
  not_pdf: "O link não trouxe um PDF que dê para ler.",
  too_large: "O PDF é grande demais (mais de 20 MB) ou o texto passa de 500 mil caracteres.",
  no_text: "O PDF é uma imagem escaneada, sem texto para ler.",
  network: "A fonte oficial não respondeu em nenhuma das 5 tentativas.",
};

const WARNING_TEXT: Record<string, string> = {
  no_ementa: "Não encontrei a ementa antes do primeiro artigo.",
  no_articles: "Não encontrei nenhum artigo. O texto está completo?",
  duplicate_address: "Dois Dispositivos com o mesmo número.",
  article_out_of_order: "Artigo fora de ordem.",
  article_gap: "A numeração dos artigos pula um número.",
  orphan: "Inciso ou alínea sem artigo ou inciso acima: ficou junto do Dispositivo anterior.",
  unclosed_quote: "Aspas abertas e não fechadas: o resto do texto ficou dentro de um artigo.",
};

const STATE_LABEL: Record<St["state"], string> = {
  fetching: "lendo o texto oficial",
  ready_to_check: "a conferir",
  confirmed: "conferida",
  manual: "texto a colar",
};

function errMessage(e: unknown, fallback: string) {
  return e instanceof ApiError ? e.message : fallback;
}

export default function Structuring({ p, reload }: { p: Detail; reload: () => Promise<Detail | undefined> }) {
  const pending = p.versions.find((v) => v.state === "pending");
  const current = p.versions.find((v) => v.state === "current");
  const ps = p.pending_structuring;
  const cs = p.current_structuring;
  if (!(pending && ps) && !(current && cs)) return null;

  return (
    <section aria-labelledby="texto-versao" className="card space-y-6">
      <h2 id="texto-versao" className="text-lg font-semibold">Texto da Versão</h2>
      <p className="text-sm text-stone-700">
        O texto oficial quebrado em Dispositivos (artigos, parágrafos, incisos…). É neles que os Comentários se prendem, e o
        leitor vê o texto assim, na página da Proposição.
      </p>
      {pending && ps && <PendingStructuring key={pending.id} p={p} version={pending} st={ps} reload={reload} />}
      {current && cs && (
        <details className="border-t border-stone-200 pt-6" open={!(pending && ps)}>
          <summary className="btn-ghost min-h-11 cursor-pointer px-4">
            Ver o texto da Versão {current.seq} (vigente){cs.provisions.length > 0 && ` · ${cs.provisions.length} Dispositivos`}
          </summary>
          <div className="mt-4">
            {cs.provisions.length === 0 ? (
              <p className="text-stone-700">Sem Documento comentado: o texto não foi estruturado nesta Versão.</p>
            ) : (
              <ProvisionTree provisions={cs.provisions} />
            )}
          </div>
        </details>
      )}
    </section>
  );
}

function PendingStructuring({
  p,
  version,
  st,
  reload,
}: {
  p: Detail;
  version: Version;
  st: St;
  reload: () => Promise<Detail | undefined>;
}) {
  const { api } = useTeamSession();
  const base = `/api/team/propositions/${encodeURIComponent(p.id)}/versions/${encodeURIComponent(version.id)}/structuring`;
  const [editing, setEditing] = useState(st.state === "manual");
  // saved: o texto gravado sobre o qual o rascunho foi escrito.
  const [saved, setSaved] = useState(st.source_text);
  const [draft, setDraft] = useState(st.source_text);
  const [seenRevision, setSeenRevision] = useState(st.revision);
  const [stale, setStale] = useState(false);
  const [busy, setBusy] = useState<"save" | "confirm" | "retry" | "reload">();
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const textRef = useRef<HTMLTextAreaElement>(null);

  // A API mandou uma Estruturação nova (a busca terminou, outra pessoa
  // gravou): sem texto pendente aqui, o rascunho acompanha.
  const dirty = draft !== saved;
  if (st.revision > seenRevision) {
    setSeenRevision(st.revision);
    setSaved(st.source_text);
    if (!dirty) setDraft(st.source_text);
  }

  // Buscando o PDF: confere de novo a cada 5 s, por até 3 minutos.
  useEffect(() => {
    if (st.state !== "fetching") return;
    let n = 0;
    const t = setInterval(() => {
      n++;
      if (n > POLL_MAX) clearInterval(t);
      else void reload();
    }, POLL_MS);
    return () => clearInterval(t);
  }, [st.state, reload]);

  async function call(kind: "save" | "confirm" | "retry", fn: () => Promise<St>, done: string) {
    setBusy(kind);
    setError(undefined);
    setNotice(undefined);
    try {
      const fresh = await fn();
      setSeenRevision(fresh.revision);
      setSaved(fresh.source_text);
      setDraft(fresh.source_text);
      setStale(false);
      setNotice(done);
      if (kind !== "save") setEditing(false);
      await reload();
    } catch (e) {
      if (e instanceof ApiError && e.code === "stale_content") {
        setStale(true);
      } else {
        setError(errMessage(e, "A ação falhou. Tente de novo."));
        if (e instanceof ApiError && (e.code === "claim_taken" || e.code === "version_not_editable")) void reload();
      }
    } finally {
      setBusy(undefined);
    }
  }

  const save = (e: React.FormEvent) => {
    e.preventDefault();
    void call(
      "save",
      () => api<St>(base, { method: "PUT", body: JSON.stringify({ source_text: draft, revision: st.revision }) }),
      "Texto salvo e Dispositivos refeitos. Confira a árvore e confirme.",
    );
  };

  async function loadSaved() {
    setBusy("reload");
    try {
      const d = await reload();
      const fresh = d?.pending_structuring;
      if (fresh) {
        setSaved(fresh.source_text);
        setDraft(fresh.source_text);
        setSeenRevision(fresh.revision);
      }
      setStale(false);
    } finally {
      setBusy(undefined);
    }
  }

  // Leva o cursor do editor à linha do aviso.
  function goToLine(line: number) {
    setEditing(true);
    requestAnimationFrame(() => {
      const el = textRef.current;
      if (!el) return;
      const lines = el.value.split("\n");
      const start = lines.slice(0, Math.max(0, line - 1)).reduce((n, l) => n + l.length + 1, 0);
      const end = start + (lines[line - 1]?.length ?? 0);
      el.focus();
      el.setSelectionRange(start, end);
      // centraliza a linha no campo (altura de linha aproximada)
      const lh = parseFloat(getComputedStyle(el).lineHeight) || 20;
      el.scrollTop = Math.max(0, (line - 1) * lh - el.clientHeight / 2);
    });
  }

  const count = charCount(draft);
  const canEdit = p.can_edit_structuring;
  const who = (id?: string | null) => (id ? p.people[id] ?? "pessoa removida" : "o Nelisa");
  const sourceLines = st.source_text.split("\n");

  return (
    <div className="space-y-4">
      <p className="text-sm text-stone-600">
        Versão {version.seq} ({version.label}), em análise · <span className="font-medium text-stone-800">{STATE_LABEL[st.state]}</span>
        {st.state !== "fetching" && ` · atualizada por ${who(st.updated_by)} em ${fmtDateTime(st.updated_at)}`}
        {version.official_text_url && (
          <>
            {" · "}
            <a href={version.official_text_url} className="link" rel="noopener noreferrer">Texto oficial</a>
          </>
        )}
      </p>

      {st.state === "fetching" && (
        <p role="status" className="rounded-md border border-stone-300 bg-stone-50 px-3 py-2 text-sm text-stone-800">
          O Nelisa está buscando e lendo o PDF oficial. Costuma levar menos de um minuto; esta página atualiza sozinha.
        </p>
      )}

      {st.state === "manual" && st.failure && (
        <div className="space-y-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-3 text-sm text-amber-950">
          <p>
            {FAILURE_TEXT[st.failure]} Abra o texto oficial, copie e cole abaixo
            {st.failure === "no_text" && " (ou digite, se for curto)"}. Sem texto, a Proposição pode ser aprovada, mas o leitor
            não verá o Documento comentado.
          </p>
          {p.can_retry_structuring && (
            <button
              type="button"
              className="btn-ghost min-h-11 px-4"
              disabled={busy !== undefined}
              onClick={() => call("retry", () => api<St>(`${base}/retry`, { method: "POST" }), "Busca pedida de novo.")}
            >
              {busy === "retry" ? "Pedindo…" : "Buscar o PDF de novo"}
            </button>
          )}
        </div>
      )}

      {st.state === "ready_to_check" && (
        <p className="rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-950">
          Confira se cada artigo, parágrafo e inciso saiu no lugar certo. Se algo saiu errado, corrija o texto e refaça. A
          Versão só pode ser aprovada depois que a Estruturação for confirmada.
        </p>
      )}

      {st.state === "confirmed" && (
        <p className="text-sm text-stone-700">
          Confirmada por {who(st.confirmed_by)} em {fmtDateTime(st.confirmed_at)}. Corrigir o texto agora volta a Estruturação
          para &ldquo;a conferir&rdquo;.
        </p>
      )}

      {st.warnings.length > 0 && (
        <div className="space-y-2">
          <h3 className="font-semibold">Avisos ({st.warnings.length})</h3>
          <ul className="space-y-2 text-sm">
            {st.warnings.map((w: Warning, i) => (
              <li key={i} className="rounded-md border border-stone-200 px-3 py-2">
                <p>{WARNING_TEXT[w.code] ?? w.code}{w.address && ` (${w.address})`}</p>
                {w.line > 0 && (
                  <p className="mt-1 text-stone-600">
                    Linha {w.line}: <q className="[overflow-wrap:anywhere]">{truncate(sourceLines[w.line - 1] ?? "", 120)}</q>
                    {canEdit && (
                      <>
                        {" "}
                        <button type="button" className="link min-h-6" onClick={() => goToLine(w.line)}>
                          ir para a linha
                        </button>
                      </>
                    )}
                  </p>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div aria-live="polite" className="space-y-2">
        <FormError message={error} />
        {notice && <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">{notice}</p>}
        {stale && (
          <div role="alert" className="space-y-2 rounded-md border border-red-300 bg-red-50 px-3 py-3 text-sm text-red-900">
            <p>Outra pessoa mudou este texto depois que você abriu a página. Nada do seu texto foi gravado, e ele continua no campo.</p>
            <button type="button" className="btn-ghost min-h-11 px-4" disabled={busy !== undefined} onClick={loadSaved}>
              {busy === "reload" ? "Carregando…" : "Descartar o meu e carregar o texto salvo"}
            </button>
          </div>
        )}
      </div>

      {canEdit && st.state === "ready_to_check" && (
        <button
          type="button"
          className="btn-lg"
          disabled={busy !== undefined || dirty}
          aria-describedby={dirty ? "confirm-hint" : undefined}
          onClick={() =>
            call(
              "confirm",
              () => api<St>(`${base}/confirm`, { method: "POST", body: JSON.stringify({ revision: st.revision }) }),
              "Estruturação confirmada. Já dá para comentar os Dispositivos.",
            )
          }
        >
          {busy === "confirm" ? "Confirmando…" : `Confirmar os ${st.provisions.length} Dispositivos`}
        </button>
      )}
      {dirty && st.state === "ready_to_check" && (
        <p id="confirm-hint" className="text-sm text-stone-600">Há texto não salvo: refaça os Dispositivos antes de confirmar.</p>
      )}

      {canEdit && st.state !== "fetching" && !editing && (
        <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setEditing(true)}>
          Corrigir o texto
        </button>
      )}
      {canEdit && st.state === "fetching" && !editing && (
        <button type="button" className="btn-ghost min-h-11 px-4" onClick={() => setEditing(true)}>
          Não quero esperar: colar o texto
        </button>
      )}
      {!canEdit && st.state !== "fetching" && (
        <p className="text-sm text-stone-600">
          {p.claim && !p.claim.mine ? `${p.claim.holder_name} está com a Posse: só essa pessoa corrige o texto.` : "Só leitura."}
        </p>
      )}

      <div className={editing && canEdit ? "grid gap-6 lg:grid-cols-2" : ""}>
        {editing && canEdit && (
          <form onSubmit={save} className="space-y-2">
            <label htmlFor="source-text" className="label">
              Texto da Versão {version.seq}
            </label>
            <p id="source-text-help" className="text-sm text-stone-600">
              Um Dispositivo por linha, começando pelo marcador (&ldquo;Art. 3º&rdquo;, &ldquo;§ 1º&rdquo;, &ldquo;I –&rdquo;,
              &ldquo;a)&rdquo;). A ementa vem antes de &ldquo;O Congresso Nacional decreta:&rdquo;; a justificação depois de
              &ldquo;JUSTIFICAÇÃO&rdquo;.
            </p>
            <textarea
              id="source-text"
              ref={textRef}
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                setNotice(undefined);
              }}
              rows={24}
              spellCheck={false}
              aria-describedby="source-text-help source-text-count"
              aria-invalid={count > TEXT_MAX ? true : undefined}
              className="input font-mono text-sm leading-6"
            />
            <p id="source-text-count" className={count > TEXT_MAX ? "text-sm text-red-800" : "text-sm text-stone-600"}>
              {count.toLocaleString("pt-BR")} de {TEXT_MAX.toLocaleString("pt-BR")} caracteres
            </p>
            <div className="flex flex-wrap gap-2">
              <button type="submit" className="btn-lg" disabled={busy !== undefined || !dirty || count > TEXT_MAX || draft.trim() === ""}>
                {busy === "save" ? "Refazendo…" : "Salvar e refazer os Dispositivos"}
              </button>
              <button
                type="button"
                className="btn-ghost min-h-12 px-4"
                disabled={busy !== undefined}
                onClick={() => {
                  setDraft(saved);
                  setEditing(st.state === "manual");
                }}
              >
                {dirty ? "Descartar alterações" : "Fechar o editor"}
              </button>
            </div>
          </form>
        )}

        {st.provisions.length > 0 && (
          <div className="space-y-2">
            <h3 className="font-semibold">Dispositivos ({st.provisions.length})</h3>
            <div className={editing && canEdit ? "max-h-[40rem] overflow-y-auto rounded-md border border-stone-200 p-3" : ""}>
              <ProvisionTree provisions={st.provisions} />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function truncate(s: string, n: number) {
  return s.length > n ? `${s.slice(0, n)}…` : s;
}

const KIND_STYLE: Partial<Record<Provision["kind"], string>> = {
  heading: "pt-3 font-semibold uppercase tracking-wide text-stone-700",
  ementa: "italic",
  justification: "border-t border-stone-200 pt-3",
};

// A árvore como o leitor vai ler: recuo por nível, rótulo antes do texto.
export function ProvisionTree({ provisions }: { provisions: Provision[] }) {
  return (
    <ol className="space-y-2">
      {provisions.map((pr) => (
        <li
          key={pr.address}
          id={`d-${pr.address}`}
          className={`[overflow-wrap:anywhere] ${KIND_STYLE[pr.kind] ?? ""}`}
          style={{ paddingLeft: `${Math.min(pr.depth, 4) * 1.25}rem` }}
        >
          {pr.kind !== "heading" && (
            <span className="mr-2 inline-block rounded bg-stone-100 px-1.5 py-0.5 text-xs font-medium not-italic text-stone-700">
              {pr.label}
            </span>
          )}
          <span className="whitespace-pre-line leading-relaxed">{pr.text}</span>
        </li>
      ))}
    </ol>
  );
}
