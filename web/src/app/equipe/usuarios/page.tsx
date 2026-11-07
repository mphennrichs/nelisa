"use client";

import { useCallback, useEffect, useState } from "react";
import { ApiError, ROLE_LABEL, useTeamSession, type Role, type User } from "@/lib/team/session";
import { FormError, RequireTeam } from "@/lib/team/ui";

const ROLES: Role[] = ["editor", "reviewer", "admin"];

function UsersAdmin({ me }: { me: User }) {
  const { api } = useTeamSession();
  const [users, setUsers] = useState<User[]>();
  const [loadError, setLoadError] = useState<string>();
  const [formError, setFormError] = useState<string>();
  const [rowError, setRowError] = useState<string>();
  const [pending, setPending] = useState(false);

  const load = useCallback(async () => {
    try {
      const r = await api<{ users: User[] }>("/api/team/users");
      setUsers(r.users);
      setLoadError(undefined);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.message : "Não foi possível carregar a lista.");
    }
  }, [api]);

  useEffect(() => {
    void load();
    // só ao abrir a página
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function onCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const fd = new FormData(form);
    setPending(true);
    setFormError(undefined);
    try {
      await api("/api/team/users", {
        method: "POST",
        body: JSON.stringify({
          name: fd.get("name"),
          email: fd.get("email"),
          password: fd.get("password"),
          role: fd.get("role"),
        }),
      });
      form.reset();
      await load();
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : "Algo deu errado. Tente de novo.");
    } finally {
      setPending(false);
    }
  }

  async function changeRole(u: User, role: Role) {
    setRowError(undefined);
    try {
      await api(`/api/team/users/${encodeURIComponent(u.id)}`, { method: "PATCH", body: JSON.stringify({ role }) });
      await load();
    } catch (err) {
      setRowError(err instanceof ApiError ? err.message : "Não foi possível mudar a role.");
      await load();
    }
  }

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">Usuários da equipe</h1>

      <section aria-labelledby="lista" className="space-y-3">
        <h2 id="lista" className="text-lg font-semibold">Equipe</h2>
        <FormError message={loadError ?? rowError} />
        {!users && !loadError && <p role="status" className="text-stone-600">Carregando…</p>}
        {users && (
          <ul className="divide-y divide-stone-200 rounded-lg border border-stone-200 bg-white">
            {users.map((u) => (
              <li key={u.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className="font-medium">
                    {u.name} {u.id === me.id && <span className="text-sm font-normal text-stone-600">(você)</span>}
                  </p>
                  <p className="text-sm text-stone-600 [overflow-wrap:anywhere]">{u.email}</p>
                  {u.must_change_password && <p className="text-sm text-amber-900">Senha temporária</p>}
                </div>
                <label className="sr-only" htmlFor={`role-${u.id}`}>Role de {u.name}</label>
                <select
                  id={`role-${u.id}`}
                  value={u.role}
                  disabled={u.id === me.id}
                  onChange={(e) => changeRole(u, e.target.value as Role)}
                  className="input min-h-10 w-auto"
                >
                  {ROLES.map((r) => (
                    <option key={r} value={r}>{ROLE_LABEL[r]}</option>
                  ))}
                </select>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-labelledby="novo" className="space-y-3">
        <h2 id="novo" className="text-lg font-semibold">Cadastrar pessoa</h2>
        <form onSubmit={onCreate} className="card grid gap-4 sm:grid-cols-2">
          <div className="sm:col-span-2"><FormError message={formError} /></div>
          <div>
            <label htmlFor="name" className="label">Nome</label>
            <input id="name" name="name" required maxLength={120} autoComplete="off" className="input min-h-11 text-base" />
          </div>
          <div>
            <label htmlFor="email" className="label">E-mail</label>
            <input id="email" name="email" type="email" required autoComplete="off" className="input min-h-11 text-base" />
          </div>
          <div>
            <label htmlFor="password" className="label">Senha inicial</label>
            <input id="password" name="password" type="text" required minLength={8} autoComplete="off" className="input min-h-11 text-base" aria-describedby="pw-hint" />
            <p id="pw-hint" className="mt-1 text-sm text-stone-600">Temporária, mínimo de 8 caracteres. Passe para a pessoa por um canal seguro.</p>
          </div>
          <div>
            <label htmlFor="role" className="label">Role</label>
            <select id="role" name="role" defaultValue="editor" className="input min-h-11 text-base">
              {ROLES.map((r) => (
                <option key={r} value={r}>{ROLE_LABEL[r]}</option>
              ))}
            </select>
          </div>
          <div className="sm:col-span-2">
            <button type="submit" disabled={pending} className="btn-lg">
              {pending ? "Cadastrando…" : "Cadastrar"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

export default function UsersPage() {
  return <RequireTeam admin>{(me) => <UsersAdmin me={me} />}</RequireTeam>;
}
