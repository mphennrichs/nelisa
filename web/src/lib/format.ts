import type { ContributionKind, ContributionStatus, ProgressKind, ReviewState, Role, Sphere } from "./types";

export const SPHERE_LABEL: Record<Sphere, string> = {
  federal: "Federal",
  state: "Estadual",
  municipal: "Municipal",
};

export const PUBLIC_STATE_LABEL: Record<ReviewState, string> = {
  draft: "Criado",
  in_review: "Em análise",
  ready: "Revisado",
};

export const TEAM_STATE_LABEL: Record<ReviewState, string> = {
  draft: "Criado",
  in_review: "Em revisão",
  ready: "Pronto",
};

export const PROGRESS_LABEL: Record<ProgressKind, string> = {
  presented: "Apresentação",
  published: "Publicação oficial",
  vote_scheduled: "Votação marcada",
  voted: "Votação",
  sanctioned: "Sanção",
  promulgated: "Promulgação",
  attached: "Apensação",
  archived: "Arquivamento",
  in_force: "Entrada em vigor",
};

export const ROLE_LABEL: Record<Role, string> = {
  editor: "Editor",
  reviewer: "Revisor",
  admin: "Admin",
};

export const CONTRIBUTION_LABEL: Record<ContributionKind, string> = {
  referral: "Indicação de proposição",
  progress_update: "Atualização de tramitação",
  source_request: "Pedido de fonte oficial",
};

export const CONTRIBUTION_STATUS_LABEL: Record<ContributionStatus, string> = {
  pending: "Pendente",
  accepted: "Aceita",
  rejected: "Rejeitada",
};

export function fmtDate(iso?: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleDateString("pt-BR", { timeZone: "UTC" });
}

export function daysUntil(iso: string) {
  return Math.round((new Date(iso).getTime() - Date.now()) / 86_400_000);
}
