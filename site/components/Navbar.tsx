"use client";

import Link from "next/link";
import { motion } from "framer-motion";

export function Navbar() {
  return (
    <motion.header
      initial={{ y: -20, opacity: 0 }}
      animate={{ y: 0, opacity: 1 }}
      transition={{ duration: 0.45 }}
      className="sticky top-0 z-40 border-b border-white/10 bg-[color-mix(in_oklab,var(--fc-bg)_90%,black)]/80 backdrop-blur"
    >
      <div className="mx-auto flex h-16 w-full max-w-6xl items-center justify-between px-6">
        <Link href="#" className="font-semibold tracking-tight text-[var(--fc-text)]">
          Env-Sync
        </Link>
        <div className="flex items-center gap-4">
          <nav className="hidden items-center gap-7 text-sm text-[var(--fc-muted)] md:flex">
            <a href="#features" className="transition hover:text-[var(--fc-text)]">Features</a>
            <a href="#how" className="transition hover:text-[var(--fc-text)]">How it works</a>
            <a href="#security" className="transition hover:text-[var(--fc-text)]">Security</a>
            <Link href="/docs" className="transition hover:text-[var(--fc-text)]">Docs</Link>
          </nav>
          <a
            href="https://github.com/Aditya190803/envsync"
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-2 rounded-full bg-white px-4 py-2 text-sm font-medium text-black transition hover:bg-white/90"
            style={{ color: "#111111" }}
          >
            Star on GitHub
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" className="h-4 w-4 fill-current" aria-hidden="true">
              <path d="M12 .5a12 12 0 0 0-3.79 23.39c.6.11.82-.26.82-.58l-.01-2.04c-3.34.73-4.04-1.41-4.04-1.41-.55-1.37-1.33-1.73-1.33-1.73-1.09-.74.08-.73.08-.73 1.2.08 1.84 1.22 1.84 1.22 1.08 1.82 2.82 1.3 3.5.99.11-.77.42-1.3.76-1.59-2.66-.3-5.47-1.31-5.47-5.85 0-1.29.47-2.35 1.23-3.18-.12-.3-.53-1.5.12-3.12 0 0 1.01-.32 3.3 1.21a11.53 11.53 0 0 1 6 0c2.3-1.53 3.3-1.2 3.3-1.2.65 1.61.24 2.81.12 3.11.77.83 1.23 1.89 1.23 3.18 0 4.55-2.81 5.54-5.49 5.84.43.36.82 1.08.82 2.18l-.01 3.23c0 .32.22.7.83.58A12 12 0 0 0 12 .5z" />
            </svg>
          </a>
        </div>
      </div>
    </motion.header>
  );
}
