# Component Guidelines

## Existing composition

Use function components and typed props. [AppProviders](../../../frontend/src/app/providers/AppProviders.tsx) supplies the sole current prop example:

```tsx
export function AppProviders({ children }: { children: ReactNode }) {
```

[AppLayout](../../../frontend/src/app/layouts/AppLayout.tsx) composes HeroUI `Button`, Lucide icons and React Router `Outlet`; [LoginPage](../../../frontend/src/features/auth/LoginPage.tsx) composes HeroUI form fields and feedback; [FoundationPages](../../../frontend/src/app/router/FoundationPages.tsx) contains simple semantic page markup. There is no established reusable table/dialog implementation. Read [project context](../project-context.md) before interpreting future UI requirements as current components.

## HeroUI v3 and CSS

Pinned versions come from [package.json](../../../frontend/package.json), currently HeroUI React/styles `3.2.6`, React `19.3.1` and Tailwind `4.3.3`. Confirm actual installed exports before using component APIs. There is no HeroUI v2 provider requirement; [main.tsx](../../../frontend/src/main.tsx) and providers use QueryClientProvider only.

[globals.css](../../../frontend/src/styles/globals.css) requires this order:

```css
@import "tailwindcss";
@import "@heroui/styles";
@import "./tokens.css";
```

Use semantic classes such as `bg-background`, `text-foreground`, `text-muted`, `border-separator`, following the layout. [tokens.css](../../../frontend/src/styles/tokens.css) sets the application font and overrides HeroUI semantic colors for light mode. Avoid scattered literal colors or introducing v2 theme configuration. The provider sets the root `dark` class, `data-theme`, and `colorScheme` from preferences; current default is light, while system-following theme is a [UI recommendation](https://app.notion.com/p/401138389edc8143b487f666a58004f4), not implemented behavior.

Prefer installed HeroUI components for controls and feedback before building equivalents. For a visible hover hint on an icon button, use HeroUI v3's compound tooltip structure and keep the button's accessible name:

```tsx
<Tooltip delay={250}>
  <Tooltip.Trigger>
    <Button isIconOnly aria-label="退出登录">…</Button>
  </Tooltip.Trigger>
  <Tooltip.Content placement="bottom">退出登录</Tooltip.Content>
</Tooltip>
```

`aria-label` names the control for assistive technology; it does not create a visible tooltip. A bare `Tooltip` around `Button` without `Tooltip.Trigger` does not open. Avoid substituting a custom tooltip or a wrapper with native `title` when HeroUI's tooltip covers the interaction.

For login and other labeled forms, compose `Form`, `TextField`, `Label` and `Input`; use `Button` for submit/retry actions and `Alert` for request failures. Put `name` and `isRequired` on `TextField`, keep `autoComplete` and controlled values on `Input`, and style these components through their `className` and theme variables to preserve the screen design. Use `Chip` for the login page's informational pills and `Spinner` for session loading. A raw `input`/`button` plus copied interaction styling loses the library's validation, focus and disabled behavior.

### Light palette and dark isolation

Light colors belong in `tokens.css` under `:root:not(.dark):not([data-theme="dark"])`; the selector excludes both dark markers set by `AppProviders`. The current light foundation is a cool gray canvas (`--background: #f5f7fb`), white surfaces, navy text and cobalt accent (`--accent: #214dd8`). Components should use the semantic utilities so the palette applies consistently. The login screen has matching `--auth-*` values and its existing `.dark .auth-login` overrides; shell treatments that should change only in light mode use explicit `dark:` classes to retain their former dark appearance.

When changing theme colors, compare the authenticated shell and anonymous login page in a browser. Check that sampled dark semantic variables and shell colors remain the same before and after the change; a light screenshot alone cannot establish dark preservation.

## Accessibility and future interactions

The existing theme button uses `onPress` and an explicit changing accessible label:

```tsx
aria-label={theme === 'dark' ? '切换浅色主题' : '切换深色主题'}
```

Retain semantic headings, landmarks and labels; icons/colors alone must not carry status. Future forms need associated labels/errors and first-invalid focus; future modal drawers/dialogs need focus trapping/restoration, scroll locking and Escape behavior. Escape closes an overlay; it must not implicitly cancel a background run. These are [UI requirements](https://app.notion.com/p/401138389edc8143b487f666a58004f4), not evidence of current accessibility coverage.

Prefer CSS transitions and respect reduced motion. Motion, Table and Virtual are optional future additions per [frontend architecture](https://app.notion.com/p/401138389edc814fb54be80c599600ef). Prefer the appropriate HeroUI v3 table virtualization route when needed; do not copy v2 props or assume a table exists. Virtualization bounds rendered loaded records; pagination/cursors must bound loaded data separately.

## Wrong vs correct

Wrong: use a clickable `div`, unlabelled icon or hardcoded success color as the only status. Correct: compose accessible controls like the current Button and include text/icon status. Review keyboard, screen reader, zoom, narrow layouts and long Chinese paths when a feature is built. The present suite does not certify those interactions.
