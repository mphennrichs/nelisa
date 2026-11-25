"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useTeamSession, type User } from "./session";

/**
 * Mostra os filhos só para quem está logado (e com a role mínima); senão manda para o login.
 * Com senha temporária, a API só atende a troca de senha (#11): as outras telas levam para ela.
 */
export function RequireTeam({
  admin = false,
  passwordChange = false,
  children,
}: {
  admin?: boolean;
  /** A tela é a troca de senha, que a senha temporária alcança. */
  passwordChange?: boolean;
  children: (user: User) => React.ReactNode;
}) {
  const { state } = useTeamSession();
  const router = useRouter();
  const mustChange = state.status === "authenticated" && state.user.must_change_password;
  useEffect(() => {
    if (state.status === "anonymous") router.replace("/equipe/entrar");
    else if (mustChange && !passwordChange) router.replace("/equipe/senha");
  }, [state.status, mustChange, passwordChange, router]);

  if (state.status !== "authenticated" || (mustChange && !passwordChange)) {
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
