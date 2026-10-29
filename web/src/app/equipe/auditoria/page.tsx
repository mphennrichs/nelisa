import { redirect } from "next/navigation";
import { db, userById } from "@/lib/store";
import { requireTeamUser } from "@/lib/session";

export default async function AuditPage() {
  const u = await requireTeamUser();
  if (u.role !== "admin") redirect("/equipe");
  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Auditoria</h1>
      <p className="text-sm text-stone-500">Somente leitura. Ninguém apaga, nem o Admin.</p>
      <ul className="card divide-y divide-stone-100 text-sm">
        {db.audit.map((a, i) => (
          <li key={i} className="py-1.5">
            <span className="font-mono text-xs text-stone-500">{new Date(a.at).toLocaleString("pt-BR")}</span>{" "}
            <strong>{userById(a.userId)?.name}</strong> · {a.detail}
          </li>
        ))}
      </ul>
    </div>
  );
}
