"use client";

import { motion, useInView } from "framer-motion";
import { useEffect, useMemo, useRef, useState } from "react";

const script = `$ envsync init
Recovery phrase generated and verified.

$ envsync project create api

$ envsync set DATABASE_URL postgres://db.internal/app

$ envsync push
push complete (revision: 18)`;

type TerminalBlockProps = {
  className?: string;
};

export function TerminalBlock({ className = "" }: TerminalBlockProps) {
  const ref = useRef<HTMLDivElement | null>(null);
  const inView = useInView(ref, { once: true, amount: 0.45 });

  const [typedLen, setTypedLen] = useState(0);

  const started = inView;
  const done = typedLen >= script.length;
  const typed = useMemo(() => script.slice(0, typedLen), [typedLen]);

  useEffect(() => {
    if (!started || done) {
      return;
    }

    const nextChar = script[typedLen];
    const delay = nextChar === "\n" ? 110 : 22;

    const timer = window.setTimeout(() => {
      setTypedLen((v) => v + 1);
    }, delay);

    return () => window.clearTimeout(timer);
  }, [started, done, typedLen]);

  return (
    <motion.div
      ref={ref}
      initial={{ opacity: 0, y: 14 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.35 }}
      transition={{ duration: 0.45 }}
      className={`relative h-[420px] w-full overflow-hidden rounded-2xl border border-white/12 bg-[#0b1017] p-4 shadow-[0_28px_80px_rgba(0,0,0,0.58)] ${className}`}
    >
      <div className="relative mb-4 flex items-center justify-between border-b border-white/10 pb-3">
        <div className="flex items-center gap-2">
          <span className="h-2.5 w-2.5 rounded-full bg-[#ff6a66]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[#ffcf4a]" />
          <span className="h-2.5 w-2.5 rounded-full bg-[#5adf8a]" />
          <span className="ml-1 text-xs text-[#a8b0bf]">envsync-session</span>
        </div>
        <span className="rounded-full border border-[var(--fc-accent)]/40 bg-[var(--fc-accent)]/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-[var(--fc-accent)]">
          live demo
        </span>
      </div>
      <pre className="relative h-[calc(100%-3.1rem)] overflow-y-auto whitespace-pre-wrap break-words font-mono text-[0.92rem] leading-8 text-[#d8deea] [scrollbar-width:thin]">
        {typed}
        {started ? <span className="terminal-cursor align-[-2px]" aria-hidden="true" /> : null}
      </pre>
    </motion.div>
  );
}
