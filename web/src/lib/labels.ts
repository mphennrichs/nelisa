import type { components } from "@/lib/api/schema";

export type Chamber = components["schemas"]["Chamber"];

export const CHAMBER_LABEL: Record<string, string> = {
  camara: "Câmara dos Deputados",
  senado: "Senado Federal",
};

export function chamberLabel(c: string) {
  return CHAMBER_LABEL[c] ?? c;
}

export const REVIEW_STATE_LABEL: Record<components["schemas"]["ReviewState"], string> = {
  draft: "Criado",
  in_review: "Em revisão",
  ready: "Pronto",
};

const dateFmt = new Intl.DateTimeFormat("pt-BR", { day: "2-digit", month: "2-digit", year: "numeric", timeZone: "America/Sao_Paulo" });
const dateTimeFmt = new Intl.DateTimeFormat("pt-BR", {
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  timeZone: "America/Sao_Paulo",
});

export function fmtDate(iso?: string | null) {
  return iso ? dateFmt.format(new Date(iso)) : "—";
}

export function fmtDateTime(iso?: string | null) {
  return iso ? dateTimeFmt.format(new Date(iso)) : "—";
}

// O Senado manda situações em CAIXA ALTA ("TRANSFORMADA EM NORMA JURÍDICA");
// para ler, viram frase normal. Texto misto fica como veio.
export function officialText(s: string) {
  if (!s || s !== s.toUpperCase() || s === s.toLowerCase()) return s;
  const lower = s.toLocaleLowerCase("pt-BR");
  return lower.charAt(0).toLocaleUpperCase("pt-BR") + lower.slice(1);
}

const CHAMBER_IN: Record<string, string> = { camara: "na Câmara dos Deputados", senado: "no Senado Federal" };
const CHAMBER_THE: Record<string, string> = { camara: "A Câmara dos Deputados", senado: "O Senado Federal" };

/** "na Câmara dos Deputados" / "no Senado Federal". */
export function chamberIn(c: string) {
  return CHAMBER_IN[c] ?? `em ${chamberLabel(c)}`;
}

/** "A Câmara dos Deputados" / "O Senado Federal". */
export function chamberThe(c: string) {
  return CHAMBER_THE[c] ?? chamberLabel(c);
}

export const SPHERE_LABEL: Record<components["schemas"]["Sphere"], string> = {
  federal: "Federal",
  state: "Estadual",
  municipal: "Municipal",
};

export const VERSION_STATE_LABEL: Record<components["schemas"]["PropositionVersion"]["state"], string> = {
  pending: "Em análise",
  current: "Vigente",
  superseded: "Substituída",
};

export const ERRATUM_STATE_LABEL: Record<components["schemas"]["ErratumState"], string> = {
  pending: "Pendente",
  approved: "Aprovada",
  rejected: "Recusada",
  withdrawn: "Retirada",
};

/** Campos do conteúdo que uma Correção menor pode mudar. */
export const CONTENT_FIELD_LABEL: Record<components["schemas"]["MinorFixField"], string> = {
  summary: "Resumo",
  short_summary: "Resumo curto",
  topic_ids: "Temas",
};

/** Texto do Selo de revisão: nunca nomes, só quantas pessoas revisaram. */
export function reviewBadgeLabel(badge: number) {
  return badge >= 2 ? "Revisado por 2 pessoas" : "Revisado por 1 pessoa";
}

/** Parágrafos de um texto simples (linha em branco separa parágrafos). */
export function paragraphs(s: string) {
  return s
    .replace(/\r\n/g, "\n")
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean);
}

/** Conta caracteres como a API (code points, depois de tirar espaços das pontas). */
export function charCount(s: string) {
  return [...s.replace(/\r\n/g, "\n").trim()].length;
}
