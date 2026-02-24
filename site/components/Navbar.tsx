"use client";

import Link from "next/link";
import { motion } from "framer-motion";

const INSTALL =
  "curl -fsSL https://raw.githubusercontent.com/Aditya190803/envsync/main/install.sh | bash";

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
        <nav className="hidden items-center gap-7 text-sm text-[var(--fc-muted)] md:flex">
          <a href="#features" className="transition hover:text-[var(--fc-text)]">Features</a>
          <a href="#how" className="transition hover:text-[var(--fc-text)]">How it works</a>
          <a href="#security" className="transition hover:text-[var(--fc-text)]">Security</a>
          <Link href="/docs" className="transition hover:text-[var(--fc-text)]">Docs</Link>
        </nav>
      </div>
    </motion.header>
  );
}
