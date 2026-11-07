"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useTeamSession, type User } from "./session";

/** Mostra os filhos só para quem está logado (e com a role mínima); senão manda para o login. */
export function RequireTeam({ admin = false, children }: { admin?: boolean; children: (user: User) => React.ReactNode }) {
  const { state } = useTeamSession();
  const router = useRouter();
  useEffect(() => {
    if (state.status === "anonymous") router.replace("/equipe/entrar");
  }, [state.status, router]);

  if (state.status !== "authenticated") {
    return <p role="status" className="text-stone-600">Carregando…</p>;
  }
  if (admin && state.user.role !== "admin") {
    return <p className="card">Só Admin acessa esta página.</p>;
  }
  return <>{children(state.user)}</>;
}

export function FieldError({ id, message }: { id: string; message?: string }) {
  if (!message) return null;
  return (
    <p id={id} className="mt-1 text-sm text-red-800">
      {message}
    </p>
  );
}

export function FormError({ message }: { message?: string }) {
  if (!message) return null;
  return (
    <div role="alert" className="rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">
      {message}
    </div>
  );
}
