interface SectionHeaderProps {
  eyebrow?: string;
  title: string;
  description?: string;
}

export function SectionHeader({ eyebrow, title, description }: SectionHeaderProps) {
  return (
    <div className="mx-auto max-w-3xl text-center">
      {eyebrow ? <p className="text-xs font-semibold uppercase tracking-[0.24em] text-[var(--fc-accent)]">{eyebrow}</p> : null}
      <h2 className="mt-3 text-balance text-3xl font-semibold leading-tight text-[var(--fc-text)] sm:text-4xl">{title}</h2>
      {description ? <p className="mt-4 text-pretty text-[var(--fc-muted)]">{description}</p> : null}
    </div>
  );
}
