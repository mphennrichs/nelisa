// In-memory store for the POC. Everything resets when the server restarts.
// Domain rules live here so the screens stay dumb (in the real system they
// move to the Go API — ADR 0004).
import { seed, TOPICS } from "./seed";
import type {
  Bill,
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

export function chamberOf(bill: Bill): Chamber {
  return db.chambers.find((c) => c.id === bill.chamberId)!;
}

export function billLabel(bill: Bill) {
  return `${bill.kind} ${bill.number}/${bill.year}`;
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

export interface PublicBill extends Omit<Bill, "summary" | "shortSummary" | "topics" | "version" | "internalNotes" | "claim" | "approvals" | "summaryAuthorIds" | "aiDraftPending"> {
  summary?: string;
  shortSummary?: string;
  topics: string[];
  officialUrl: string;
  provisions?: Bill["version"]["provisions"];
  comments?: { provisionId: string; text: string }[];
  reviewers: 1 | 2;
}

export function toPublic(bill: Bill): PublicBill | null {
  if (bill.state === "draft") return null;
  const ready = bill.state === "ready";
  return {
    id: bill.id,
    kind: bill.kind,
    chamberId: bill.chamberId,
    number: bill.number,
    year: bill.year,
    ementa: bill.ementa,
    authors: bill.authors,
    state: bill.state,
    createdAt: bill.createdAt,
    pendingSecondReview: bill.pendingSecondReview,
    progress: bill.progress,
    rollCall: bill.rollCall,
    aiDraftUsed: false,
    officialUrl: bill.version.officialUrl,
    topics: ready ? bill.topics : [],
    summary: ready ? bill.summary : undefined,
    shortSummary: ready ? bill.shortSummary : undefined,
    provisions: ready ? bill.version.provisions : undefined,
    comments: ready ? bill.version.comments.map((c) => ({ provisionId: c.provisionId, text: c.text })) : undefined,
    reviewers: bill.pendingSecondReview ? 1 : 2,
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

export function publicBills(f: PublicFilter): PublicBill[] {
  const q = f.q?.trim().toLowerCase();
  return db.bills
    .map(toPublic)
    .filter((b): b is PublicBill => b !== null)
    .filter((b) => {
      const c = db.chambers.find((x) => x.id === b.chamberId)!;
      if (f.sphere && c.sphere !== f.sphere) return false;
      if (f.uf && c.uf !== f.uf) return false;
      if (f.chamberId && c.id !== f.chamberId) return false;
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

export function nextVote(bill: Bill): string | undefined {
  const t = now();
  return bill.progress
    .filter((p) => p.kind === "vote_scheduled" && p.date >= t)
    .map((p) => p.date)
    .sort()[0];
}

export function claimOf(bill: Bill) {
  if (!bill.claim) return undefined;
  if (Date.now() - new Date(bill.claim.lastActivity).getTime() > CLAIM_TTL_MS) return undefined;
  return bill.claim;
}

export interface QueueItem {
  kind: "bill" | "contribution" | "second_review";
  sphere: Sphere;
  vote?: string;
  createdAt: string;
  bill?: Bill;
  contribution?: Contribution;
}

// Sphere always wins; inside a sphere, nearest vote first; then oldest.
export function reviewQueue(sphere?: string): QueueItem[] {
  const items: QueueItem[] = [];
  for (const b of db.bills) {
    const sph = chamberOf(b).sphere;
    if (b.state !== "ready") items.push({ kind: "bill", sphere: sph, vote: nextVote(b), createdAt: b.createdAt, bill: b });
    else if (b.pendingSecondReview) items.push({ kind: "second_review", sphere: sph, vote: nextVote(b), createdAt: b.createdAt, bill: b });
  }
  for (const c of db.contributions.filter((c) => c.status === "pending")) {
    const bill = c.billId ? db.bills.find((b) => b.id === c.billId) : undefined;
    const sph = bill ? chamberOf(bill).sphere : sphereOfUrl(c.url) ?? "municipal";
    items.push({ kind: "contribution", sphere: sph, vote: bill ? nextVote(bill) : undefined, createdAt: c.createdAt, bill, contribution: c });
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

function getBill(id: string) {
  const b = db.bills.find((x) => x.id === id);
  if (!b) throw new DomainError("PL não encontrado.");
  return b;
}

function requireClaim(bill: Bill, user: User) {
  if (bill.state === "ready") throw new DomainError("PL pronto não é editado: correções passam por Errata.");
  const c = claimOf(bill);
  if (!c || c.userId !== user.id) throw new DomainError("Assuma o PL antes de editar.");
  c.lastActivity = now();
}

export function claimBill(billId: string, user: User) {
  const bill = getBill(billId);
  if (bill.state === "ready") throw new DomainError("PL pronto não é editado: correções passam por Errata.");
  const c = claimOf(bill);
  if (c && c.userId !== user.id) throw new DomainError(`PL já está com ${userById(c.userId)?.name}.`);
  bill.claim = { userId: user.id, lastActivity: now() };
}

export function releaseClaim(billId: string, user: User) {
  const bill = getBill(billId);
  const c = claimOf(bill);
  if (!c) return;
  if (c.userId !== user.id) requireRole(user, "reviewer", "admin");
  bill.claim = undefined;
  audit(user.id, "release_claim", `${billLabel(bill)}: posse liberada`);
}

export function saveSummary(billId: string, user: User, data: { summary: string; shortSummary: string; topics: string[] }) {
  const bill = getBill(billId);
  requireClaim(bill, user);
  if (data.shortSummary.length > 280) throw new DomainError("O resumo curto passa de 280 caracteres.");
  if (data.topics.length > 3) throw new DomainError("No máximo 3 temas.");
  bill.summary = data.summary;
  bill.shortSummary = data.shortSummary;
  bill.topics = data.topics;
  addSummaryAuthor(bill, user);
  bill.aiDraftPending = false;
  if (bill.state === "draft") bill.state = "in_review";
}

function addSummaryAuthor(bill: Bill, user: User) {
  if (!bill.summaryAuthorIds.includes(user.id)) bill.summaryAuthorIds.push(user.id);
}

export function aiDraft(billId: string, user: User) {
  const bill = getBill(billId);
  requireClaim(bill, user);
  // Fake: no external call in the POC.
  bill.summary = `[Rascunho de IA, edite antes de aprovar] ${bill.ementa} O texto tem ${bill.version.provisions.length} dispositivos.`;
  bill.shortSummary = bill.ementa.slice(0, 200);
  bill.topics = [TOPICS[bill.number % TOPICS.length]];
  addSummaryAuthor(bill, user);
  bill.aiDraftUsed = true;
  bill.aiDraftPending = true;
  audit(user.id, "ai_draft", `${billLabel(bill)}: rascunho de IA gerado`);
}

export function addComment(billId: string, user: User, provisionId: string, text: string) {
  const bill = getBill(billId);
  requireClaim(bill, user);
  if (!text.trim()) throw new DomainError("Comentário vazio.");
  bill.version.comments.push({ id: newId("c"), provisionId, text, authorId: user.id });
}

export function addInternalNote(billId: string, user: User, text: string) {
  const bill = getBill(billId);
  if (!text.trim()) throw new DomainError("Nota vazia.");
  bill.internalNotes.push({ id: newId("n"), authorId: user.id, at: now(), text });
}

export function markReady(billId: string, user: User) {
  const bill = getBill(billId);
  requireRole(user, "reviewer", "admin");
  if (bill.state === "ready") throw new DomainError("PL já está pronto.");
  if (!bill.summary.trim() || !bill.shortSummary.trim()) throw new DomainError("Falta resumo ou resumo curto.");
  if (bill.topics.length === 0) throw new DomainError("Escolha ao menos um tema.");
  if (bill.aiDraftPending) throw new DomainError("O rascunho de IA precisa ser editado e salvo por uma pessoa antes de aprovar.");
  const self = bill.summaryAuthorIds.includes(user.id);
  if (self && user.role !== "admin") throw new DomainError("Quatro olhos: você escreveu o resumo, outro revisor precisa aprovar.");
  bill.state = "ready";
  bill.claim = undefined;
  bill.pendingSecondReview = self;
  bill.approvals.push({ userId: user.id, at: now(), bypass: self });
  audit(user.id, "approve", `${billLabel(bill)} aprovado${self ? " SEM segunda revisão" : ""}`);
}

export function confirmSecondReview(billId: string, user: User) {
  const bill = getBill(billId);
  requireRole(user, "reviewer", "admin");
  if (!bill.pendingSecondReview) throw new DomainError("Não há segunda revisão pendente.");
  if (bill.summaryAuthorIds.includes(user.id) || bill.approvals.some((a) => a.userId === user.id)) {
    throw new DomainError("A segunda revisão precisa ser de outra pessoa.");
  }
  bill.pendingSecondReview = false;
  bill.approvals.push({ userId: user.id, at: now(), bypass: false });
  audit(user.id, "second_review", `${billLabel(bill)}: segunda revisão confirmada`);
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
  billId?: string;
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
    const existing = db.bills.find((b) => b.version.officialUrl === input.url);
    if (existing) throw new DomainError(`Esse PL já está no Nelisa: ${billLabel(existing)}.`);
  }
  if (input.kind === "progress_update" && (!input.billId || !input.progressKind || !input.date)) {
    throw new DomainError("Escolha o PL, o tipo de evento e a data.");
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
  const bill = accept && c.kind === "progress_update" ? getBill(c.billId ?? "") : undefined;
  c.status = accept ? "accepted" : "rejected";
  if (bill) {
    bill.progress.push({ id: newId("p"), date: c.date!, kind: c.progressKind!, description: c.note || "Atualização enviada por leitor" });
    bill.progress.sort((a, b) => a.date.localeCompare(b.date));
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
