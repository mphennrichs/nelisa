"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";
import * as store from "@/lib/store";
import { DomainError } from "@/lib/store";
import { requireTeamUser, requireUser, SESSION_COOKIE } from "@/lib/session";
import type { ContributionKind, ProgressKind, Role } from "@/lib/types";

function str(fd: FormData, key: string) {
  return String(fd.get(key) ?? "").trim();
}

function withParam(path: string, key: string, value: string) {
  const sep = path.includes("?") ? "&" : "?";
  return `${path}${sep}${key}=${encodeURIComponent(value)}`;
}

// Runs a domain operation; domain errors go back to the form as ?erro=.
async function run(back: string, fn: () => void | Promise<void>, next = back) {
  try {
    await fn();
  } catch (e) {
    if (e instanceof DomainError) redirect(withParam(back, "erro", e.message));
    throw e;
  }
  revalidatePath("/", "layout");
  redirect(next);
}

// --- Session ---------------------------------------------------------------

export async function signIn(fd: FormData) {
  let target = "/equipe";
  try {
    const u = store.login(str(fd, "email"), String(fd.get("password") ?? ""));
    (await cookies()).set(SESSION_COOKIE, u.id, { httpOnly: true, sameSite: "strict", path: "/" });
    if (u.mustChangePassword) target = "/equipe/senha";
  } catch (e) {
    if (e instanceof DomainError) redirect(withParam("/entrar", "erro", e.message));
    throw e;
  }
  redirect(target);
}

export async function signOut() {
  (await cookies()).delete(SESSION_COOKIE);
  redirect("/");
}

export async function changePassword(fd: FormData) {
  const u = await requireUser();
  await run("/equipe/senha", () => store.changeOwnPassword(u, String(fd.get("password") ?? "")), "/equipe");
}

// --- Proposition editing ----------------------------------------------------------

export async function claimProposition(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.claimProposition(id, u));
}

export async function releaseClaim(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.releaseClaim(id, u));
}

export async function saveSummary(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () =>
    store.saveSummary(id, u, {
      summary: str(fd, "summary"),
      shortSummary: str(fd, "shortSummary"),
      topics: fd.getAll("topics").map(String),
    }),
  );
}

export async function aiDraft(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.aiDraft(id, u));
}

export async function addComment(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.addComment(id, u, str(fd, "provisionId"), str(fd, "text")));
}

export async function addInternalNote(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.addInternalNote(id, u, str(fd, "text")));
}

export async function markReady(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.markReady(id, u));
}

export async function confirmSecondReview(fd: FormData) {
  const u = await requireTeamUser();
  const id = str(fd, "propositionId");
  await run(`/equipe/proposicao/${id}`, () => store.confirmSecondReview(id, u));
}

// --- Contributions ---------------------------------------------------------

export async function submitContribution(fd: FormData) {
  const kind = str(fd, "kind") as ContributionKind;
  let code = "";
  try {
    code = store.submitContribution({
      kind,
      source: str(fd, "source"),
      url: str(fd, "url") || undefined,
      propositionId: str(fd, "propositionId") || undefined,
      progressKind: (str(fd, "progressKind") || undefined) as ProgressKind | undefined,
      date: str(fd, "date") ? new Date(`${str(fd, "date")}T12:00:00Z`).toISOString() : undefined,
      domain: str(fd, "domain") || undefined,
      note: str(fd, "note") || undefined,
    }).trackingCode;
  } catch (e) {
    if (e instanceof DomainError) redirect(withParam(`/contribuir?tipo=${kind}`, "erro", e.message));
    throw e;
  }
  redirect(`/contribuicao?codigo=${code}&novo=1`);
}

export async function resolveContribution(fd: FormData) {
  const u = await requireTeamUser();
  await run("/equipe/contribuicoes", () =>
    store.resolveContribution(str(fd, "code"), u, str(fd, "decision") === "accept", str(fd, "chamberId") || undefined),
  );
}

// --- Admin -----------------------------------------------------------------

export async function createUser(fd: FormData) {
  const u = await requireTeamUser();
  await run("/equipe/usuarios", () =>
    store.createUser(u, {
      name: str(fd, "name"),
      email: str(fd, "email").toLowerCase(),
      password: String(fd.get("password") ?? ""),
      role: str(fd, "role") as Role,
    }),
  );
}

export async function resetPassword(fd: FormData) {
  const u = await requireTeamUser();
  await run("/equipe/usuarios", () => store.resetPassword(u, str(fd, "userId"), String(fd.get("password") ?? "")));
}

export async function setActive(fd: FormData) {
  const u = await requireTeamUser();
  await run("/equipe/usuarios", () => store.setActive(u, str(fd, "userId"), str(fd, "active") === "1"));
}
