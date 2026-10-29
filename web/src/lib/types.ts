// Code names follow docs/nomes-no-codigo.md.

export type Sphere = "federal" | "state" | "municipal";
export type ReviewState = "draft" | "in_review" | "ready";
export type Role = "editor" | "reviewer" | "admin";

export interface Chamber {
  id: string;
  name: string;
  sphere: Sphere;
  uf?: string;
  city?: string;
  hasApi: boolean;
}

export interface OfficialSource {
  domain: string;
  chamberId: string;
}

export interface Term {
  chamberId: string;
  party: string;
  uf: string;
  start: string;
  end?: string;
}

export interface Legislator {
  id: string;
  name: string;
  terms: Term[];
  officialContact?: string;
}

export type Author =
  | { kind: "legislator"; legislatorId: string }
  | { kind: "body"; name: string }
  | { kind: "cosigners"; count: number };

export type ProgressKind =
  | "presented"
  | "published"
  | "vote_scheduled"
  | "voted"
  | "sanctioned"
  | "promulgated"
  | "attached"
  | "archived"
  | "in_force";

export interface ProgressEvent {
  id: string;
  date: string;
  kind: ProgressKind;
  description: string;
}

// As published by the source; vote is the source's own label ("Sim", "Não",
// "Abstenção", "P-NRV"...). legislatorId only when we have the Legislator.
export interface RollCallVote {
  name: string;
  party: string;
  uf: string;
  vote: string;
  legislatorId?: string;
}

export interface Provision {
  id: string;
  label: string;
  text: string;
}

export interface Comment {
  id: string;
  provisionId: string;
  text: string;
  authorId: string;
}

export interface InternalNote {
  id: string;
  authorId: string;
  at: string;
  text: string;
}

export interface Version {
  id: string;
  officialUrl: string;
  createdAt: string;
  provisions: Provision[];
  comments: Comment[];
}

export interface Approval {
  userId: string;
  at: string;
  bypass: boolean;
}

export interface Claim {
  userId: string;
  lastActivity: string;
}

export interface Proposition {
  id: string;
  // Type of proposição as the source names it: PL, PEC, PLP...
  kind: string;
  // Chambers it went through, in order; the first is the origin chamber.
  chamberIds: string[];
  // Apensação: id of the principal Proposition this one was attached to.
  attachedTo?: string;
  // Prejudicada: stopped without a vote because of another Proposition.
  superseded?: boolean;
  number: number;
  year: number;
  ementa: string;
  authors: Author[];
  state: ReviewState;
  createdAt: string;
  version: Version;
  summary: string;
  shortSummary: string;
  topics: string[];
  // Everyone who wrote the Summary: none of them may approve it (four eyes).
  summaryAuthorIds: string[];
  approvals: Approval[];
  pendingSecondReview: boolean;
  progress: ProgressEvent[];
  rollCall?: { description: string; votes: RollCallVote[] };
  claim?: Claim;
  internalNotes: InternalNote[];
  aiDraftUsed: boolean;
  // AI draft generated and not yet saved by a person.
  aiDraftPending?: boolean;
}

export type ContributionKind = "referral" | "progress_update" | "source_request";
export type ContributionStatus = "pending" | "accepted" | "rejected";

export interface Contribution {
  trackingCode: string;
  kind: ContributionKind;
  status: ContributionStatus;
  createdAt: string;
  source: string;
  propositionId?: string;
  url?: string;
  progressKind?: ProgressKind;
  date?: string;
  domain?: string;
  note?: string;
}

export interface User {
  id: string;
  name: string;
  email: string;
  password: string;
  role: Role;
  active: boolean;
  mustChangePassword: boolean;
}

export interface AuditEntry {
  at: string;
  userId: string;
  action: string;
  detail: string;
}
