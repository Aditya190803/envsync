import type { ReactNode } from "react";

interface FeatureCardProps {
  icon: ReactNode;
  title: string;
  description: string;
}

export function FeatureCard({ icon, title, description }: FeatureCardProps) {
  return (
    <article className="group rounded-2xl border border-white/10 bg-[linear-gradient(180deg,rgba(255,255,255,0.03),rgba(255,255,255,0.015))] p-6 shadow-[0_12px_40px_rgba(0,0,0,0.28)] transition duration-300 hover:-translate-y-0.5 hover:border-[var(--fc-accent)]/50 hover:shadow-[0_16px_48px_rgba(255,122,26,0.14)]">
      <div className="mb-4 h-px w-16 bg-[linear-gradient(90deg,var(--fc-accent),transparent)]" />
      <div className="mb-4 inline-flex h-10 w-10 items-center justify-center rounded-xl border border-[var(--fc-accent)]/30 bg-[var(--fc-accent)]/10 text-[var(--fc-accent)]">
        {icon}
      </div>
      <h3 className="text-lg font-semibold text-[var(--fc-text)]">{title}</h3>
      <p className="mt-2 text-sm leading-relaxed text-[var(--fc-muted)]">{description}</p>
    </article>
  );
}
