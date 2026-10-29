// Fake session for the POC: a cookie with the user id. The real system uses
// server-side sessions in Postgres + TOTP (ADR 0001).
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { userById } from "./store";

export const SESSION_COOKIE = "nelisa_uid";

export async function currentUser() {
  const id = (await cookies()).get(SESSION_COOKIE)?.value;
  const u = userById(id);
  return u?.active ? u : undefined;
}

export async function requireUser() {
  const u = await currentUser();
  if (!u) redirect("/entrar");
  return u;
}

// Team pages: also force the password change after admin-set passwords.
export async function requireTeamUser() {
  const u = await requireUser();
  if (u.mustChangePassword) redirect("/equipe/senha");
  return u;
}
