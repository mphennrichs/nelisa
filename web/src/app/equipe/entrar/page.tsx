"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ApiError, useTeamSession } from "@/lib/team/session";
import { FormError } from "@/lib/team/ui";

export default function SignIn() {
  const { state, login } = useTeamSession();
  const router = useRouter();
  const [error, setError] = useState<string>();
  const [pending, setPending] = useState(false);

  useEffect(() => {
    if (state.status === "authenticated") router.replace("/equipe/fila");
  }, [state.status, router]);

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    setPending(true);
    setError(undefined);
    try {
      await login(String(fd.get("email")), String(fd.get("password")));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Algo deu errado. Tente de novo.");
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="mx-auto max-w-sm space-y-5">
      <h1 className="text-2xl font-bold">Entrar na equipe</h1>
      <form onSubmit={onSubmit} className="card space-y-4" noValidate={false}>
        <FormError message={error} />
        <div>
          <label htmlFor="email" className="label">E-mail</label>
          <input id="email" name="email" type="email" autoComplete="username" required className="input min-h-11 text-base" />
        </div>
        <div>
          <label htmlFor="password" className="label">Senha</label>
          <input id="password" name="password" type="password" autoComplete="current-password" required className="input min-h-11 text-base" />
        </div>
        <button type="submit" disabled={pending} className="btn-lg w-full">
          {pending ? "Entrando…" : "Entrar"}
        </button>
      </form>
      <p className="text-sm text-stone-600">
        Não tem conta? Quem cadastra a equipe é o Admin. Primeira instalação?{" "}
        <Link href="/equipe/primeiro-acesso" className="link">Criar o primeiro Admin</Link>.
      </p>
    </div>
  );
}
