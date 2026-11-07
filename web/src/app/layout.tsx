import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Nelisa",
  description: "Proposições legislativas explicadas, com revisão humana.",
  // Beta discreto: nada é indexado até o lançamento (docs/plano.md).
  robots: { index: false, follow: false },
};

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="pt-BR">
      <body className="min-h-dvh antialiased">
        <a href="#conteudo" className="skip-link">Pular para o conteúdo</a>
        {children}
      </body>
    </html>
  );
}
