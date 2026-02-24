"use client";

import { motion, useInView } from "framer-motion";
import { useEffect, useMemo, useRef, useState } from "react";

const script = `$ envsync init

Recovery phrase generated and verified.

$ envsync project create api

$ envsync set DATABASE_URL postgres://db.internal/app

$ envsync push

push complete (revision: 18)`;

export function TerminalBlock() {
  const ref = useRef<HTMLDivElement | null>(null);
  const inView = useInView(ref, { once: true, amount: 0.45 });

  const [started, setStarted] = useState(false);
  const [typedLen, setTypedLen] = useState(0);

  const done = typedLen >= script.length;
  const typed = useMemo(() => script.slice(0, typedLen), [typedLen]);

  useEffect(() => {
    if (inView && !started) {
      setStarted(true);
    }
  }, [inView, started]);

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
      className="rounded-2xl border border-white/10 bg-[#0f1319] p-4 shadow-[0_20px_70px_rgba(0,0,0,0.5)]"
    >
      <div className="mb-4 flex items-center gap-2">
        <span className="h-2.5 w-2.5 rounded-full bg-[#ff6a66]" />
        <span className="h-2.5 w-2.5 rounded-full bg-[#ffcf4a]" />
        <span className="h-2.5 w-2.5 rounded-full bg-[#5adf8a]" />
      </div>
      <pre className="min-h-[220px] whitespace-pre-wrap break-words font-mono text-sm leading-7 text-[#d8deea]">
        {typed}
        {started ? <span className="terminal-cursor align-[-2px]" aria-hidden="true" /> : null}
      </pre>
    </motion.div>
  );
}
