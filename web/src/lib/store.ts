// In-memory store for the POC. Everything resets when the server restarts.
// Domain rules live here so the screens stay dumb (in the real system they
// move to the Go API — ADR 0004).
import { seed, TOPICS } from "./seed";
import type {
  Proposition,
  Chamber,
  Contribution,
  ContributionKind,
  ProgressKind,
  Role,
  Sphere,
  User,
} from "./types";

type Db = ReturnType<typeof seed>;

const g = globalThis as unknown as { __nelisaDb?: Db };
export const db: Db = (g.__nelisaDb ??= seed());

export { TOPICS };

export class DomainError extends Error {}

const SPHERE_RANK: Record<Sphere, number> = { federal: 0, state: 1, municipal: 2 };
const CLAIM_TTL_MS = 3 * 24 * 60 * 60 * 1000;

function now() {
  return new Date().toISOString();
}

function newId(prefix: string) {
  return `${prefix}-${Math.random().toString(36).slice(2, 8)}`;
}

export function chamberOf(proposition: Proposition): Chamber {
  return db.chambers.find((c) => c.id === proposition.chamberIds[0])!;
}

// All chambers it went through, origin first.
export function chambersOf(p: { chamberIds: string[] }): Chamber[] {
  return p.chamberIds.map((id) => db.chambers.find((c) => c.id === id)!);
}

export function propositionLabel(proposition: Proposition) {
  return `${proposition.kind} ${proposition.number}/${proposition.year}`;
}

export function userById(id: string | undefined) {
  return db.users.find((u) => u.id === id);
}

export function audit(userId: string, action: string, detail: string) {
  db.audit.unshift({ at: now(), userId, action, detail });
}

// --- Public visibility -----------------------------------------------------
// Rule: what comes from outside (official facts) is always public; what the
// Nelisa produces (summary, comments, topics) only shows once ready.

export interface PublicProposition extends Omit<Proposition, "summary" | "shortSummary" | "topics" | "version" | "internalNotes" | "claim" | "approvals" | "summaryAuthorIds" | "aiDraftPending"> {
  summary?: string;
  shortSummary?: string;
  topics: string[];
  officialUrl: string;
  provisions?: Proposition["version"]["provisions"];
  comments?: { provisionId: string; text: string }[];
  reviewers: 1 | 2;
}

export function toPublic(proposition: Proposition): PublicProposition | null {
  if (proposition.state === "draft") return null;
  const ready = proposition.state === "ready";
  return {
    id: proposition.id,
    kind: proposition.kind,
    chamberIds: proposition.chamberIds,
    attachedTo: proposition.attachedTo,
    superseded: proposition.superseded,
    number: proposition.number,
    year: proposition.year,
    ementa: proposition.ementa,
    authors: proposition.authors,
    state: proposition.state,
    createdAt: proposition.createdAt,
    pendingSecondReview: proposition.pendingSecondReview,
    progress: proposition.progress,
    rollCall: proposition.rollCall,
    aiDraftUsed: false,
    officialUrl: proposition.version.officialUrl,
    topics: ready ? proposition.topics : [],
    summary: ready ? proposition.summary : undefined,
    shortSummary: ready ? proposition.shortSummary : undefined,
    provisions: ready ? proposition.version.provisions : undefined,
    comments: ready ? proposition.version.comments.map((c) => ({ provisionId: c.provisionId, text: c.text })) : undefined,
    reviewers: proposition.pendingSecondReview ? 1 : 2,
  };
}

export interface PublicFilter {
  q?: string;
  sphere?: string;
  uf?: string;
  chamberId?: string;
  topic?: string;
  state?: string;
}

export function publicPropositions(f: PublicFilter): PublicProposition[] {
  const q = f.q?.trim().toLowerCase();
  return db.propositions
    .map(toPublic)
    .filter((b): b is PublicProposition => b !== null)
    .filter((b) => {
      const cs = chambersOf(b);
      if (f.sphere && cs[0].sphere !== f.sphere) return false;
      if (f.uf && cs[0].uf !== f.uf) return false;
      if (f.chamberId && !b.chamberIds.includes(f.chamberId)) return false;
      if (f.topic && !b.topics.includes(f.topic)) return false;
      if (f.state && b.state !== f.state) return false;
      if (q) {
        const hay = `${b.kind} ${b.number}/${b.year} ${b.ementa} ${b.summary ?? ""}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    })
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}

// --- Review queue ----------------------------------------------------------

export function nextVote(proposition: Proposition): string | undefined {
  const t = now();
  return proposition.progress
    .filter((p) => p.kind === "vote_scheduled" && p.date >= t)
    .map((p) => p.date)
    .sort()[0];
}

export function claimOf(proposition: Proposition) {
  if (!proposition.claim) return undefined;
  if (Date.now() - new Date(proposition.claim.lastActivity).getTime() > CLAIM_TTL_MS) return undefined;
  return proposition.claim;
}

export interface QueueItem {
  kind: "proposition" | "contribution" | "second_review";
  sphere: Sphere;
  vote?: string;
  createdAt: string;
  proposition?: Proposition;
  contribution?: Contribution;
}

// Sphere always wins; inside a sphere, nearest vote first; then oldest.
export function reviewQueue(sphere?: string): QueueItem[] {
  const items: QueueItem[] = [];
  for (const b of db.propositions) {
    const sph = chamberOf(b).sphere;
    if (b.state !== "ready") items.push({ kind: "proposition", sphere: sph, vote: nextVote(b), createdAt: b.createdAt, proposition: b });
    else if (b.pendingSecondReview) items.push({ kind: "second_review", sphere: sph, vote: nextVote(b), createdAt: b.createdAt, proposition: b });
  }
  for (const c of db.contributions.filter((c) => c.status === "pending")) {
    const proposition = c.propositionId ? db.propositions.find((b) => b.id === c.propositionId) : undefined;
    const sph = proposition ? chamberOf(proposition).sphere : sphereOfUrl(c.url) ?? "municipal";
    items.push({ kind: "contribution", sphere: sph, vote: proposition ? nextVote(proposition) : undefined, createdAt: c.createdAt, proposition, contribution: c });
  }
  return items
    .filter((i) => !sphere || i.sphere === sphere)
    .sort((a, b) => {
      const s = SPHERE_RANK[a.sphere] - SPHERE_RANK[b.sphere];
      if (s) return s;
      if (a.vote && b.vote) return a.vote.localeCompare(b.vote);
      if (a.vote) return -1;
      if (b.vote) return 1;
      return a.createdAt.localeCompare(b.createdAt);
    });
}

// --- Editing rules ---------------------------------------------------------

function requireRole(user: User, ...roles: Role[]) {
  if (!roles.includes(user.role)) throw new DomainError("Seu papel não permite essa ação.");
}

function getProposition(id: string) {
  const b = db.propositions.find((x) => x.id === id);
  if (!b) throw new DomainError("Proposição não encontrada.");
  return b;
}

function requireClaim(proposition: Proposition, user: User) {
  if (proposition.state === "ready") throw new DomainError("Proposição pronta não é editada: correções passam por Errata.");
  const c = claimOf(proposition);
  if (!c || c.userId !== user.id) throw new DomainError("Assuma a proposição antes de editar.");
  c.lastActivity = now();
}

export function claimProposition(propositionId: string, user: User) {
  const proposition = getProposition(propositionId);
  if (proposition.state === "ready") throw new DomainError("Proposição pronta não é editada: correções passam por Errata.");
  const c = claimOf(proposition);
  if (c && c.userId !== user.id) throw new DomainError(`A proposição já está com ${userById(c.userId)?.name}.`);
  proposition.claim = { userId: user.id, lastActivity: now() };
}

export function releaseClaim(propositionId: string, user: User) {
  const proposition = getProposition(propositionId);
  const c = claimOf(proposition);
  if (!c) return;
  if (c.userId !== user.id) requireRole(user, "reviewer", "admin");
  proposition.claim = undefined;
  audit(user.id, "release_claim", `${propositionLabel(proposition)}: posse liberada`);
}

export function saveSummary(propositionId: string, user: User, data: { summary: string; shortSummary: string; topics: string[] }) {
  const proposition = getProposition(propositionId);
  requireClaim(proposition, user);
  if (data.shortSummary.length > 280) throw new DomainError("O resumo curto passa de 280 caracteres.");
  if (data.topics.length > 3) throw new DomainError("No máximo 3 temas.");
  proposition.summary = data.summary;
  proposition.shortSummary = data.shortSummary;
  proposition.topics = data.topics;
  addSummaryAuthor(proposition, user);
  proposition.aiDraftPending = false;
  if (proposition.state === "draft") proposition.state = "in_review";
}

function addSummaryAuthor(proposition: Proposition, user: User) {
  if (!proposition.summaryAuthorIds.includes(user.id)) proposition.summaryAuthorIds.push(user.id);
}

export function aiDraft(propositionId: string, user: User) {
  const proposition = getProposition(propositionId);
  requireClaim(proposition, user);
  // Fake: no external call in the POC.
  proposition.summary = `[Rascunho de IA, edite antes de aprovar] ${proposition.ementa} O texto tem ${proposition.version.provisions.length} dispositivos.`;
  proposition.shortSummary = proposition.ementa.slice(0, 200);
  proposition.topics = [TOPICS[proposition.number % TOPICS.length]];
  addSummaryAuthor(proposition, user);
  proposition.aiDraftUsed = true;
  proposition.aiDraftPending = true;
  audit(user.id, "ai_draft", `${propositionLabel(proposition)}: rascunho de IA gerado`);
}

export function addComment(propositionId: string, user: User, provisionId: string, text: string) {
  const proposition = getProposition(propositionId);
  requireClaim(proposition, user);
  if (!text.trim()) throw new DomainError("Comentário vazio.");
  proposition.version.comments.push({ id: newId("c"), provisionId, text, authorId: user.id });
}

export function addInternalNote(propositionId: string, user: User, text: string) {
  const proposition = getProposition(propositionId);
  if (!text.trim()) throw new DomainError("Nota vazia.");
  proposition.internalNotes.push({ id: newId("n"), authorId: user.id, at: now(), text });
}

export function markReady(propositionId: string, user: User) {
  const proposition = getProposition(propositionId);
  requireRole(user, "reviewer", "admin");
  if (proposition.state === "ready") throw new DomainError("A proposição já está pronta.");
  if (!proposition.summary.trim() || !proposition.shortSummary.trim()) throw new DomainError("Falta resumo ou resumo curto.");
  if (proposition.topics.length === 0) throw new DomainError("Escolha ao menos um tema.");
  if (proposition.aiDraftPending) throw new DomainError("O rascunho de IA precisa ser editado e salvo por uma pessoa antes de aprovar.");
  const self = proposition.summaryAuthorIds.includes(user.id);
  if (self && user.role !== "admin") throw new DomainError("Quatro olhos: você escreveu o resumo, outro revisor precisa aprovar.");
  proposition.state = "ready";
  proposition.claim = undefined;
  proposition.pendingSecondReview = self;
  proposition.approvals.push({ userId: user.id, at: now(), bypass: self });
  audit(user.id, "approve", `${propositionLabel(proposition)} aprovado${self ? " SEM segunda revisão" : ""}`);
}

export function confirmSecondReview(propositionId: string, user: User) {
  const proposition = getProposition(propositionId);
  requireRole(user, "reviewer", "admin");
  if (!proposition.pendingSecondReview) throw new DomainError("Não há segunda revisão pendente.");
  if (proposition.summaryAuthorIds.includes(user.id) || proposition.approvals.some((a) => a.userId === user.id)) {
    throw new DomainError("A segunda revisão precisa ser de outra pessoa.");
  }
  proposition.pendingSecondReview = false;
  proposition.approvals.push({ userId: user.id, at: now(), bypass: false });
  audit(user.id, "second_review", `${propositionLabel(proposition)}: segunda revisão confirmada`);
}

// --- Contributions ---------------------------------------------------------

export function domainOf(url: string | undefined) {
  try {
    if (!url) return undefined;
    const u = new URL(url);
    if (u.protocol !== "http:" && u.protocol !== "https:") return undefined;
    return u.hostname.replace(/^www\d*\./, "");
  } catch {
    return undefined;
  }
}

export function isOfficialDomain(domain: string) {
  if (domain.endsWith(".leg.br") || domain.endsWith(".gov.br")) return true;
  return db.officialSources.some((s) => domain === s.domain || domain.endsWith(`.${s.domain}`));
}

function sphereOfUrl(url?: string): Sphere | undefined {
  const d = domainOf(url);
  const src = d && db.officialSources.find((s) => d === s.domain || d.endsWith(`.${s.domain}`));
  return src ? db.chambers.find((c) => c.id === src.chamberId)?.sphere : undefined;
}

// Rejects bare suffixes ("com.br", "org") that would whitelist everyone.
function isRegistrableDomain(domain?: string) {
  if (!domain || !/^[a-z0-9.-]+$/.test(domain)) return false;
  const labels = domain.split(".");
  return domain.endsWith(".br") ? labels.length >= 3 : labels.length >= 2;
}

function trackingCode() {
  return `NL-${Math.random().toString(36).slice(2, 6).toUpperCase()}`;
}

export function submitContribution(input: {
  kind: ContributionKind;
  source: string;
  url?: string;
  propositionId?: string;
  progressKind?: ProgressKind;
  date?: string;
  domain?: string;
  note?: string;
}): Contribution {
  if (!domainOf(input.source)) throw new DomainError("Informe a fonte como um link válido.");
  if (input.kind === "referral") {
    const d = domainOf(input.url);
    if (!d) throw new DomainError("Endereço inválido.");
    if (!isOfficialDomain(d)) throw new DomainError(`O domínio ${d} não é reconhecido como fonte oficial. Você pode pedir o cadastro dele.`);
    const existing = db.propositions.find((b) => b.version.officialUrl === input.url);
    if (existing) throw new DomainError(`Essa proposição já está no Nelisa: ${propositionLabel(existing)}.`);
  }
  if (input.kind === "progress_update" && (!input.propositionId || !input.progressKind || !input.date)) {
    throw new DomainError("Escolha a proposição, o tipo de evento e a data.");
  }
  if (input.kind === "source_request" && !isRegistrableDomain(input.domain)) throw new DomainError("Informe um domínio válido, como camaradexyz.com.br.");
  const c: Contribution = { ...input, trackingCode: trackingCode(), status: "pending", createdAt: now() };
  db.contributions.push(c);
  return c;
}

export function resolveContribution(code: string, user: User, accept: boolean, chamberId?: string) {
  const c = db.contributions.find((x) => x.trackingCode === code);
  if (!c || c.status !== "pending") throw new DomainError("Contribuição não está pendente.");
  if (accept && c.kind === "source_request" && !db.chambers.some((ch) => ch.id === chamberId)) {
    throw new DomainError("Escolha a Casa legislativa dona do domínio.");
  }
  const proposition = accept && c.kind === "progress_update" ? getProposition(c.propositionId ?? "") : undefined;
  c.status = accept ? "accepted" : "rejected";
  if (proposition) {
    proposition.progress.push({ id: newId("p"), date: c.date!, kind: c.progressKind!, description: c.note || "Atualização enviada por leitor" });
    proposition.progress.sort((a, b) => a.date.localeCompare(b.date));
  }
  if (accept && c.kind === "source_request" && c.domain) {
    db.officialSources.push({ domain: c.domain, chamberId: chamberId! });
  }
  audit(user.id, accept ? "accept_contribution" : "reject_contribution", `${c.trackingCode} (${c.kind})`);
}

// --- Team ------------------------------------------------------------------

export function login(email: string, password: string) {
  const u = db.users.find((x) => x.email === email.trim().toLowerCase() && x.password === password);
  if (!u || !u.active) throw new DomainError("E-mail ou senha inválidos.");
  return u;
}

export function createUser(admin: User, data: { name: string; email: string; password: string; role: Role }) {
  requireRole(admin, "admin");
  data = { ...data, email: data.email.trim().toLowerCase() };
  if (!["editor", "reviewer", "admin"].includes(data.role)) throw new DomainError("Papel inválido.");
  if (data.password.length < 8) throw new DomainError("Senha com no mínimo 8 caracteres.");
  if (db.users.some((u) => u.email === data.email)) throw new DomainError("E-mail já cadastrado.");
  const u: User = { id: newId("u"), ...data, active: true, mustChangePassword: true };
  db.users.push(u);
  audit(admin.id, "create_user", `${u.name} (${u.role})`);
}

export function resetPassword(admin: User, userId: string, password: string) {
  requireRole(admin, "admin");
  if (password.length < 8) throw new DomainError("Senha com no mínimo 8 caracteres.");
  const u = userById(userId);
  if (!u) throw new DomainError("Usuário não encontrado.");
  u.password = password;
  u.mustChangePassword = true;
  audit(admin.id, "reset_password", u.name);
}

export function setActive(admin: User, userId: string, active: boolean) {
  requireRole(admin, "admin");
  const u = userById(userId);
  if (!u) throw new DomainError("Usuário não encontrado.");
  if (u.id === admin.id) throw new DomainError("Você não pode desativar a si mesmo.");
  u.active = active;
  audit(admin.id, active ? "activate_user" : "deactivate_user", u.name);
}

export function changeOwnPassword(user: User, password: string) {
  if (password.length < 8) throw new DomainError("Senha com no mínimo 8 caracteres.");
  user.password = password;
  user.mustChangePassword = false;
}
