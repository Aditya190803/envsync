import { FeatureCard } from "@/components/FeatureCard";
import { Footer } from "@/components/Footer";
import { Hero } from "@/components/Hero";
import { Navbar } from "@/components/Navbar";
import { SectionHeader } from "@/components/SectionHeader";
import { TerminalBlock } from "@/components/TerminalBlock";

const features = [
  {
    title: "AES-256-GCM encryption",
    description: "Secrets are encrypted before local or remote writes, with phrase-derived keys using Argon2id.",
  },
  {
    title: "Explicit push and pull",
    description: "You decide exactly when synchronization happens, with no background surprises.",
  },
  {
    title: "Machine restore",
    description: "Onboard a second machine from your recovery phrase and remote encrypted state.",
  },
  {
    title: "Conflict-aware sync",
    description: "Detect conflicts, choose override behavior, and preserve deterministic outcomes.",
  },
  {
    title: "Doctor diagnostics",
    description: "Run fast health checks for config, active project state, and remote connectivity.",
  },
  {
    title: "Structured audit logs",
    description: "Every important action is append-logged in JSON for inspection and automation.",
  },
];

const steps = [
  {
    title: "Initialize and scope",
    detail: "Create your encrypted local vault, select project, and choose environment.",
  },
  {
    title: "Manage secrets safely",
    detail: "Set, get, list, rollback, and review history without exposing plaintext by default.",
  },
  {
    title: "Sync with control",
    detail: "Push and pull to file, HTTP, or Convex backends with optimistic concurrency checks.",
  },
];

function BoltIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true">
      <path d="M13 2 4 14h7l-1 8 10-14h-7l0-6Z" />
    </svg>
  );
}

export default function Home() {
  return (
    <div className="min-h-screen bg-[var(--fc-bg)] text-[var(--fc-text)]">
      <Navbar />
      <main>
        <Hero />

        <section className="mx-auto w-full max-w-6xl px-6 pb-6">
          <div className="grid gap-4 rounded-2xl border border-white/10 bg-white/[0.02] p-4 sm:grid-cols-3">
            <div className="rounded-xl border border-white/10 bg-black/20 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">Encryption</p>
              <p className="mt-2 text-2xl font-semibold">AES-256-GCM</p>
            </div>
            <div className="rounded-xl border border-white/10 bg-black/20 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">Sync model</p>
              <p className="mt-2 text-2xl font-semibold">Explicit Push/Pull</p>
            </div>
            <div className="rounded-xl border border-white/10 bg-black/20 p-4">
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--fc-muted)]">Remote safety</p>
              <p className="mt-2 text-2xl font-semibold">Revision-checked</p>
            </div>
          </div>
        </section>

        <section id="features" className="mx-auto w-full max-w-6xl px-6 py-20">
          <SectionHeader
            eyebrow="Capabilities"
            title="Everything needed for reliable secret workflows"
            description="Built for teams that want strong cryptography, clean ergonomics, and explicit operational control."
          />
          <div className="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {features.map((feature) => (
              <FeatureCard key={feature.title} icon={<BoltIcon />} title={feature.title} description={feature.description} />
            ))}
          </div>
        </section>

        <section id="how" className="border-y border-white/10 bg-white/[0.02] py-20">
          <div className="mx-auto w-full max-w-6xl px-6">
            <SectionHeader
              eyebrow="How it works"
              title="Simple flow. Strong guarantees."
              description="Three clear steps from local setup to secure cloud backup."
            />
            <div className="mt-12 grid gap-6 md:grid-cols-3">
              {steps.map((step, index) => (
                <article key={step.title} className="rounded-2xl border border-white/10 bg-black/20 p-6">
                  <p className="text-sm font-semibold text-[var(--fc-accent)]">0{index + 1}</p>
                  <h3 className="mt-3 text-xl font-semibold">{step.title}</h3>
                  <p className="mt-3 text-[var(--fc-muted)]">{step.detail}</p>
                </article>
              ))}
            </div>
          </div>
        </section>

        <section className="mx-auto grid w-full max-w-6xl gap-10 px-6 py-20 lg:grid-cols-2">
          <div>
            <SectionHeader
              eyebrow="Terminal demo"
              title="Built for people who live in the shell"
              description="Fast command surface, deterministic behavior, and output made for automation."
            />
          </div>
          <TerminalBlock />
        </section>

        <section id="security" className="mx-auto w-full max-w-6xl px-6 py-20">
          <SectionHeader
            eyebrow="Security"
            title="No hidden plaintext paths"
            description="Phrase-derived keys, encrypted values, version history, and audit trails all designed for high-signal operations."
          />
          <div className="mt-10 grid gap-6 md:grid-cols-2">
            <article className="rounded-2xl border border-white/10 bg-black/25 p-6">
              <h3 className="text-lg font-semibold">Security model</h3>
              <ul className="mt-4 space-y-2 text-sm text-[var(--fc-muted)]">
                <li>Argon2id key derivation from recovery phrase</li>
                <li>AES-256-GCM encryption for secret values</li>
                <li>Remote writes validated by optimistic `revision` checks</li>
              </ul>
            </article>
            <article className="rounded-2xl border border-white/10 bg-black/25 p-6">
              <h3 className="text-lg font-semibold">Remote integrations</h3>
              <ul className="mt-4 space-y-2 text-sm text-[var(--fc-muted)]">
                <li>HTTP remote backend with bearer token support</li>
                <li>Convex backup transport with deploy key + API key modes</li>
                <li>Restore command for second-machine recovery</li>
              </ul>
            </article>
          </div>
        </section>

        <section className="mx-auto w-full max-w-6xl px-6 py-20">
          <div className="rounded-3xl border border-[var(--fc-accent)]/35 bg-[linear-gradient(120deg,rgba(255,122,26,0.2),rgba(255,122,26,0.05))] p-10 text-center shadow-[0_24px_80px_rgba(255,122,26,0.15)]">
            <h2 className="text-3xl font-semibold sm:text-4xl">Ship with confidence. Keep secret operations explicit.</h2>
            <p className="mx-auto mt-4 max-w-2xl text-[var(--fc-muted)]">
              Start with one install command and move to reliable encrypted sync across every environment your team touches.
            </p>
            <a
              href="#home"
              className="mt-8 inline-flex rounded-full bg-[var(--fc-accent)] px-8 py-3 text-sm font-semibold text-[#2b1708] transition hover:brightness-110"
            >
              Install Env-Sync
            </a>
          </div>
        </section>
      </main>
      <Footer />
    </div>
  );
}
