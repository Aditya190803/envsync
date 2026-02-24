# Env-Sync site

Marketing site and docs UI for Env-Sync, built with Next.js App Router + Tailwind CSS v4.

## Stack

- Next.js 16
- React 19
- TypeScript
- Tailwind CSS v4
- Framer Motion

## Local development

From `site/`:

```bash
npm install
npm run dev
```

Open `http://localhost:3000`.

## Scripts

```bash
npm run dev    # start local dev server
npm run build  # production build
npm run start  # run built app
npm run lint   # eslint
```

## Routes

- `/` landing page
- `/docs` product documentation page

## Project structure

```text
app/
  page.tsx          # landing page
  docs/page.tsx     # docs page
  layout.tsx        # global metadata/fonts
  globals.css       # global styles + theme vars
components/
  Hero.tsx
  Navbar.tsx
  Footer.tsx
  FeatureCard.tsx
  SectionHeader.tsx
  TerminalBlock.tsx
```

## Deployment

Standard Next.js deployment flow works (Vercel or self-hosted Node runtime):

```bash
npm run build
npm run start
```
