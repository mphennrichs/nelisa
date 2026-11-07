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
