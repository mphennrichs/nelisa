import Link from "next/link";
import { signOut } from "@/app/actions";
import { requireUser } from "@/lib/session";
import { ROLE_LABEL } from "@/lib/format";

export default async function TeamLayout({ children }: { children: React.ReactNode }) {
  const u = await requireUser();
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-4 rounded-lg bg-stone-900 px-4 py-2 text-sm text-stone-100">
        <Link href="/equipe" className="hover:underline">Fila de revisão</Link>
        <Link href="/equipe/contribuicoes" className="hover:underline">Contribuições</Link>
        {u.role === "admin" && <Link href="/equipe/usuarios" className="hover:underline">Usuários</Link>}
        {u.role === "admin" && <Link href="/equipe/auditoria" className="hover:underline">Auditoria</Link>}
        <span className="flex-1" />
        <span className="text-stone-400">{u.name} · {ROLE_LABEL[u.role]}</span>
        <form action={signOut}><button className="hover:underline">Sair</button></form>
      </div>
      {children}
    </div>
  );
}
