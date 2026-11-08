"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError } from "@/lib/team/ui";

// Só funciona uma vez: com BOOTSTRAP_TOKEN configurado e nenhum usuário
// cadastrado (a API garante as duas coisas).
export default function Bootstrap() {
  const { bootstrap } = useTeamSession();
  const router = useRouter();
  const [error, setError] = useState<string>();
  const [pending, setPending] = useState(false);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    setPending(true);
    setError(undefined);
    try {
      await bootstrap({
        token: String(fd.get("token")),
        name: String(fd.get("name")),
        email: String(fd.get("email")),
        password: String(fd.get("password")),
      });
      router.replace("/equipe/fila");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Algo deu errado. Tente de novo.");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="mx-auto max-w-sm space-y-5">
      <h1 className="text-2xl font-bold">Criar o primeiro Admin</h1>
      <p className="text-stone-700">
        Usado uma única vez, na instalação. Depois disso, novas pessoas são cadastradas pelo Admin.
      </p>
      <form onSubmit={onSubmit} className="card space-y-4">
        <FormError message={error} />
        <div>
          <label htmlFor="token" className="label">Token de instalação</label>
          <input id="token" name="token" type="password" autoComplete="off" required className="input min-h-11 text-base" aria-describedby="token-hint" />
          <p id="token-hint" className="mt-1 text-sm text-stone-600">O valor de BOOTSTRAP_TOKEN configurado no servidor.</p>
        </div>
        <div>
          <label htmlFor="name" className="label">Nome</label>
          <input id="name" name="name" autoComplete="name" required maxLength={120} className="input min-h-11 text-base" />
        </div>
        <div>
          <label htmlFor="email" className="label">E-mail</label>
          <input id="email" name="email" type="email" autoComplete="username" required className="input min-h-11 text-base" />
        </div>
        <div>
          <label htmlFor="password" className="label">Senha</label>
          <input id="password" name="password" type="password" autoComplete="new-password" required minLength={8} className="input min-h-11 text-base" aria-describedby="pw-hint" />
          <p id="pw-hint" className="mt-1 text-sm text-stone-600">Pelo menos 8 caracteres.</p>
        </div>
        <button type="submit" disabled={pending} className="btn-lg w-full">
          {pending ? "Criando…" : "Criar Admin e entrar"}
        </button>
      </form>
    </div>
  );
}
