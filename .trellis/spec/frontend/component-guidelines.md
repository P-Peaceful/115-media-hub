# Component Guidelines

## Existing composition

Use function components and typed props. [AppProviders](../../../frontend/src/app/providers/AppProviders.tsx) supplies the sole current prop example:

```tsx
export function AppProviders({ children }: { children: ReactNode }) {
```

[AppLayout](../../../frontend/src/app/layouts/AppLayout.tsx) composes HeroUI `Button`, Lucide icons and React Router `Outlet`; [FoundationPages](../../../frontend/src/app/router/FoundationPages.tsx) contains simple semantic page markup. There is no established reusable form/table/dialog implementation. Read [project context](../project-context.md) before interpreting future UI requirements as current components.

## HeroUI v3 and CSS

Pinned versions come from [package.json](../../../frontend/package.json), currently HeroUI React/styles `3.2.6`, React `19.3.1` and Tailwind `4.3.3`. Confirm actual installed exports before using component APIs. There is no HeroUI v2 provider requirement; [main.tsx](../../../frontend/src/main.tsx) and providers use QueryClientProvider only.

[globals.css](../../../frontend/src/styles/globals.css) requires this order:

```css
@import "tailwindcss";
@import "@heroui/styles";
@import "./tokens.css";
```

Use semantic classes such as `bg-background`, `text-foreground`, `text-muted`, `border-separator`, following the layout. [tokens.css](../../../frontend/src/styles/tokens.css) currently sets the application font while inheriting HeroUI semantic colors. Avoid scattered literal colors or introducing v2 theme configuration. The provider sets the root `dark` class, `data-theme`, and `colorScheme` from preferences; current default is light, while system-following theme is a [UI recommendation](https://app.notion.com/p/401138389edc8143b487f666a58004f4), not implemented behavior.

## Accessibility and future interactions

The existing theme button uses `onPress` and an explicit changing accessible label:

```tsx
aria-label={theme === 'dark' ? '切换浅色主题' : '切换深色主题'}
```

Retain semantic headings, landmarks and labels; icons/colors alone must not carry status. Future forms need associated labels/errors and first-invalid focus; future modal drawers/dialogs need focus trapping/restoration, scroll locking and Escape behavior. Escape closes an overlay; it must not implicitly cancel a background run. These are [UI requirements](https://app.notion.com/p/401138389edc8143b487f666a58004f4), not evidence of current accessibility coverage.

Prefer CSS transitions and respect reduced motion. Motion, Table and Virtual are optional future additions per [frontend architecture](https://app.notion.com/p/401138389edc814fb54be80c599600ef). Prefer the appropriate HeroUI v3 table virtualization route when needed; do not copy v2 props or assume a table exists. Virtualization bounds rendered loaded records; pagination/cursors must bound loaded data separately.

## Wrong vs correct

Wrong: use a clickable `div`, unlabelled icon or hardcoded success color as the only status. Correct: compose accessible controls like the current Button and include text/icon status. Review keyboard, screen reader, zoom, narrow layouts and long Chinese paths when a feature is built. The present suite does not certify those interactions.
