"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { ROLE_LABEL, useTeamSession } from "@/lib/team/session";

export default function TeamNav() {
  const { state, logout } = useTeamSession();
  const pathname = usePathname();
  const router = useRouter();
  const links = [
    { href: "/equipe", label: "Início" },
    { href: "/equipe/proposicoes", label: "Proposições" },
    ...(state.status === "authenticated" && state.user.role === "admin" ? [{ href: "/equipe/usuarios", label: "Usuários" }] : []),
    { href: "/equipe/senha", label: "Trocar senha" },
  ];
  return (
    <header className="border-b border-stone-200 bg-white">
      <nav aria-label="Equipe" className="mx-auto flex max-w-3xl flex-wrap items-center gap-x-5 gap-y-2 px-4 py-3 text-sm">
        <span className="text-lg font-bold">
          Nelisa <span className="font-normal text-stone-600">equipe</span>
        </span>
        {state.status === "authenticated" &&
          links.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              aria-current={pathname === l.href ? "page" : undefined}
              className={pathname === l.href ? "font-semibold underline underline-offset-4" : "link"}
            >
              {l.label}
            </Link>
          ))}
        <span className="flex-1" />
        {state.status === "authenticated" && (
          <>
            <span className="text-stone-600">
              {state.user.name} · {ROLE_LABEL[state.user.role]}
            </span>
            <button
              type="button"
              className="btn-ghost min-h-10"
              onClick={async () => {
                await logout();
                router.replace("/equipe/entrar");
              }}
            >
              Sair
            </button>
          </>
        )}
      </nav>
    </header>
  );
}
