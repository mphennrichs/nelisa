"use client";

import { useEffect, useRef, useState } from "react";

// Curated palette with no party association (no red, no green-yellow, no
// saturated blue/orange). To be reviewed by the team before launch.
const GRADIENTS = [
  { name: "Grafite", from: "#2b2f36", to: "#4a505b", ink: "#f4f1ec" },
  { name: "Petróleo", from: "#1f3b45", to: "#2f5d68", ink: "#eef4f4" },
  { name: "Ameixa", from: "#3b2a3d", to: "#5e4560", ink: "#f5eef5" },
  { name: "Chumbo", from: "#34404e", to: "#56677a", ink: "#eef1f5" },
  { name: "Areia", from: "#e9e2d0", to: "#c9bfa5", ink: "#2b2a26" },
  { name: "Névoa", from: "#dfe4ea", to: "#b8c2cc", ink: "#22282e" },
  { name: "Lavanda", from: "#d9d4e7", to: "#b3abc9", ink: "#262236" },
  { name: "Pérola", from: "#f4f1ec", to: "#ddd6cc", ink: "#26241f" },
];

const W = 1080;
const H = 1920;

function wrap(ctx: CanvasRenderingContext2D, text: string, maxWidth: number) {
  const lines: string[] = [];
  let line = "";
  for (const word of text.split(/\s+/)) {
    const next = line ? `${line} ${word}` : word;
    if (ctx.measureText(next).width > maxWidth && line) {
      lines.push(line);
      line = word;
    } else line = next;
  }
  if (line) lines.push(line);
  return lines;
}

export default function ShareStory({ text, url }: { text: string; url: string }) {
  const [open, setOpen] = useState(false);
  const [g, setG] = useState(0);
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const ctx = canvas.current?.getContext("2d");
    if (!ctx) return;
    const grad = GRADIENTS[g];
    const bg = ctx.createLinearGradient(0, 0, W, H);
    bg.addColorStop(0, grad.from);
    bg.addColorStop(1, grad.to);
    ctx.fillStyle = bg;
    ctx.fillRect(0, 0, W, H);
    ctx.fillStyle = grad.ink;
    ctx.font = "600 64px system-ui, sans-serif";
    const lines = wrap(ctx, text, W - 200);
    const lh = 88;
    let y = H / 2 - (lines.length * lh) / 2;
    for (const l of lines) {
      ctx.fillText(l, 100, y);
      y += lh;
    }
    ctx.font = "500 40px system-ui, sans-serif";
    ctx.globalAlpha = 0.8;
    ctx.fillText(url, 100, H - 160);
    ctx.globalAlpha = 1;
  }, [g, text, url, open]);

  function download() {
    const a = document.createElement("a");
    a.href = canvas.current!.toDataURL("image/png");
    a.download = "nelisa-story.png";
    a.click();
  }

  if (!open) {
    return <button className="btn-ghost" onClick={() => setOpen(true)}>Compartilhar como story</button>;
  }

  return (
    <div className="flex flex-wrap gap-4 rounded-lg border border-stone-200 p-3">
      <canvas ref={canvas} width={W} height={H} className="h-80 w-[180px] rounded-md shadow" />
      <div className="space-y-3">
        <div className="label">Fundo</div>
        <div className="grid grid-cols-4 gap-2">
          {GRADIENTS.map((x, i) => (
            <button
              key={x.name}
              title={x.name}
              onClick={() => setG(i)}
              className={`h-10 w-10 rounded-md ${i === g ? "ring-2 ring-stone-900 ring-offset-2" : ""}`}
              style={{ background: `linear-gradient(135deg, ${x.from}, ${x.to})` }}
            />
          ))}
        </div>
        <div className="flex gap-2">
          <button className="btn" onClick={download}>Baixar imagem</button>
          <button className="btn-ghost" onClick={() => setOpen(false)}>Fechar</button>
        </div>
      </div>
    </div>
  );
}
