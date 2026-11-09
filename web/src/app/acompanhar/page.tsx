import type { Metadata } from "next";
import LookupForm from "./LookupForm";
import PublicShell from "@/components/PublicShell";

export const metadata: Metadata = {
  title: "Acompanhar contribuição · Nelisa",
  description: "Consulte a avaliação da sua Contribuição pelo código de acompanhamento.",
};

export default function TrackPage() {
  return (
    <PublicShell>
      <div className="space-y-6">
        <div className="space-y-2">
          <h1 className="text-3xl font-bold">Acompanhar contribuição</h1>
          <p className="text-lg text-stone-700">Digite o código que apareceu quando você mandou a Contribuição.</p>
        </div>
        <LookupForm />
      </div>
    </PublicShell>
  );
}
