// Documento comentado (público): o texto da Versão vigente, Dispositivo por
// Dispositivo, com os Comentários do Nelisa logo abaixo de cada um. Cada
// Dispositivo tem âncora (#art-3.par-2) para compartilhar. A API só manda
// Comentários aprovados, sem nomes.
import type { PublicPropositionDetail } from "@/lib/public/api";
import { paragraphs } from "@/lib/labels";

type Provision = NonNullable<PublicPropositionDetail["annotated_text"]>[number];

// Acima disso, o texto inteiro fica recolhido e os comentados vêm primeiro.
const LONG = 60;

function anchor(address: string) {
  return `d-${address}`;
}

function Comments({ comments }: { comments: Provision["comments"] }) {
  if (comments.length === 0) return null;
  return (
    <div className="mt-2 space-y-2">
      {comments.map((c, i) => (
        <aside key={i} className="rounded-md border-l-4 border-sky-700 bg-sky-50 px-4 py-3 text-base text-stone-900">
          <p className="mb-1 text-sm font-semibold text-sky-900">Comentário do Nelisa</p>
          <div className="space-y-2 leading-relaxed">
            {paragraphs(c.body).map((p, j) => (
              <p key={j} className="whitespace-pre-line [overflow-wrap:anywhere]">{p}</p>
            ))}
          </div>
        </aside>
      ))}
    </div>
  );
}

function ProvisionItem({ p, withId = true }: { p: Provision; withId?: boolean }) {
  if (p.kind === "heading") {
    return (
      <li id={withId ? anchor(p.address) : undefined} className="pt-4 text-sm font-semibold uppercase tracking-wide text-stone-700">
        {p.text}
      </li>
    );
  }
  if (p.kind === "justification") {
    return (
      <li id={withId ? anchor(p.address) : undefined} className="border-t border-stone-200 pt-4">
        <details>
          <summary className="cursor-pointer py-2 font-semibold">Justificação de quem propôs</summary>
          <div className="mt-2 space-y-3 leading-relaxed text-stone-800">
            {paragraphs(p.text).map((para, i) => (
              <p key={i} className="[overflow-wrap:anywhere]">{para}</p>
            ))}
          </div>
        </details>
        <Comments comments={p.comments} />
      </li>
    );
  }
  return (
    <li
      id={withId ? anchor(p.address) : undefined}
      className={`scroll-mt-4 ${p.kind === "ementa" ? "italic" : ""}`}
      style={{ marginLeft: `${Math.min(p.depth, 4) * 1}rem` }}
    >
      <p className="leading-relaxed [overflow-wrap:anywhere]">
        {p.kind !== "preamble" && p.kind !== "ementa" ? (
          <a href={`#${anchor(p.address)}`} className="mr-1 text-stone-500 no-underline hover:underline" aria-label={`Link para ${p.label}`}>
            #
          </a>
        ) : null}
        {p.text}
      </p>
      <Comments comments={p.comments} />
    </li>
  );
}

export default function AnnotatedText({ provisions, officialUrl }: { provisions: Provision[]; officialUrl: string }) {
  const commented = provisions.filter((p) => p.comments.length > 0);
  const long = provisions.length > LONG;
  const source = officialUrl ? (
    <a href={officialUrl} className="link" rel="noopener noreferrer">texto oficial</a>
  ) : (
    "texto oficial"
  );

  return (
    <section aria-labelledby="documento" className="space-y-4">
      <h2 id="documento" className="text-lg font-semibold">Documento comentado</h2>
      <p className="max-w-prose text-sm text-stone-700">
        O texto da proposta, artigo por artigo, com os comentários do Nelisa. É uma cópia para leitura; em caso de dúvida,
        vale o {source}.
      </p>

      {long && commented.length > 0 && (
        <div className="space-y-3">
          <h3 className="font-semibold">Trechos comentados ({commented.length})</h3>
          <ol className="max-w-prose space-y-4">
            {commented.map((p) => (
              <li key={p.address}>
                <p className="text-sm font-medium text-stone-600">
                  <a href={`#${anchor(p.address)}`} className="link">{p.label}</a>
                </p>
                <p className="leading-relaxed [overflow-wrap:anywhere]">{p.kind === "justification" ? "Justificação" : p.text}</p>
                <Comments comments={p.comments} />
              </li>
            ))}
          </ol>
        </div>
      )}

      {long ? (
        <details className="rounded-md border border-stone-200 px-4 py-2">
          <summary className="min-h-11 cursor-pointer py-2 font-semibold">Ler o texto inteiro ({provisions.length} trechos)</summary>
          <ol className="mt-2 max-w-prose space-y-3 pb-2">
            {provisions.map((p) => (
              <ProvisionItem key={p.address} p={p} />
            ))}
          </ol>
        </details>
      ) : (
        <ol className="max-w-prose space-y-3">
          {provisions.map((p) => (
            <ProvisionItem key={p.address} p={p} />
          ))}
        </ol>
      )}
    </section>
  );
}
