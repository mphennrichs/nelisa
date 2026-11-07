"use client";

import { useState } from "react";
import type { components } from "@/lib/api/schema";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";

export default function ChangePassword() {
  const { api, setSession } = useTeamSession();
  const [error, setError] = useState<string>();
  const [done, setDone] = useState(false);
  const [pending, setPending] = useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const fd = new FormData(form);
    if (fd.get("new_password") !== fd.get("confirm")) {
      setError("A confirmação não é igual à nova senha.");
      return;
    }
    setPending(true);
    setError(undefined);
    setDone(false);
    try {
      const s = await api<components["schemas"]["Session"]>("/api/team/me/password", {
        method: "POST",
        body: JSON.stringify({ current_password: fd.get("current_password"), new_password: fd.get("new_password") }),
      });
      setSession(s);
      form.reset();
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Algo deu errado. Tente de novo.");
    } finally {
      setPending(false);
    }
  }

  return (
    <RequireTeam>
      {() => (
        <div className="mx-auto max-w-sm space-y-5">
          <h1 className="text-2xl font-bold">Trocar senha</h1>
          <form onSubmit={onSubmit} className="card space-y-4">
            <FormError message={error} />
            {done && (
              <p role="status" className="rounded-md border border-emerald-300 bg-emerald-50 px-3 py-2 text-sm text-emerald-900">
                Senha trocada. Outras sessões abertas foram encerradas.
              </p>
            )}
            <div>
              <label htmlFor="current_password" className="label">Senha atual</label>
              <input id="current_password" name="current_password" type="password" autoComplete="current-password" required className="input min-h-11 text-base" />
            </div>
            <div>
              <label htmlFor="new_password" className="label">Nova senha</label>
              <input id="new_password" name="new_password" type="password" autoComplete="new-password" required minLength={8} className="input min-h-11 text-base" aria-describedby="np-hint" />
              <p id="np-hint" className="mt-1 text-sm text-stone-600">Pelo menos 8 caracteres.</p>
            </div>
            <div>
              <label htmlFor="confirm" className="label">Confirme a nova senha</label>
              <input id="confirm" name="confirm" type="password" autoComplete="new-password" required className="input min-h-11 text-base" />
            </div>
            <button type="submit" disabled={pending} className="btn-lg w-full">
              {pending ? "Salvando…" : "Trocar senha"}
            </button>
          </form>
        </div>
      )}
    </RequireTeam>
  );
}
