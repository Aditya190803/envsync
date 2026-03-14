# ENV Sync site

Marketing site and docs UI for ENV Sync, built with Next.js App Router + Tailwind CSS v4.

## Stack

- Next.js 16
- React 19
- TypeScript
- Tailwind CSS v4
- Framer Motion
- Stack Auth
- Convex

## Local development

From `site/`:

```bash
npm install
npm run dev
```

Open `http://localhost:3000`.

Create a local site env file before testing auth and dashboard routes:

```bash
cp env.example .env
```

Set these env vars before testing auth and dashboard routes:

```bash
NEXT_PUBLIC_STACK_PROJECT_ID=
NEXT_PUBLIC_STACK_PUBLISHABLE_CLIENT_KEY=
STACK_SECRET_SERVER_KEY=
NEXT_PUBLIC_STACK_BASE_URL=

NEXT_PUBLIC_CONVEX_URL=
```

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
- `/handler/*` Stack Auth handler routes (sign in/up/callback pages)
- `/dashboard` authenticated shell (v1 scaffold)
- `/dashboard/devices` device approval and revocation scaffold

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
