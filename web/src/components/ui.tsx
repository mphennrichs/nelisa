import type { ReactNode } from "react";

export function ErrorBanner({ message }: { message?: string }) {
  if (!message) return null;
  return <div className="mb-4 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-800">{message}</div>;
}

const TONES = {
  neutral: "bg-stone-100 text-stone-700",
  green: "bg-emerald-100 text-emerald-800",
  amber: "bg-amber-100 text-amber-900",
  blue: "bg-sky-100 text-sky-800",
};

export function Badge({ children, tone = "neutral" }: { children: ReactNode; tone?: keyof typeof TONES }) {
  return <span className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${TONES[tone]}`}>{children}</span>;
}

export function Hidden({ name, value }: { name: string; value: string }) {
  return <input type="hidden" name={name} value={value} />;
}
