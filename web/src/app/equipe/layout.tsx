import type { Metadata } from "next";
import { TeamSessionProvider } from "@/lib/team/session";
import TeamNav from "./TeamNav";

export const metadata: Metadata = { title: { default: "Equipe", template: "%s · Equipe Nelisa" } };

export default function TeamLayout({ children }: { children: React.ReactNode }) {
  return (
    <TeamSessionProvider>
      <TeamNav />
      <main id="conteudo" className="mx-auto max-w-3xl px-4 py-8">{children}</main>
    </TeamSessionProvider>
  );
}
