# Component Guidelines

## Composition and props

Write function components with typed props and pure rendering. Keep business orchestration in feature hooks/pages and reusable presentation in components. State belongs with the nearest owner. Prefer composition over a generic component with many unrelated mode flags; extract only cohesive pieces. Follow the [Rules of React](https://react.dev/reference/rules).

Illustrative plain React component (not an existing file):

```tsx
import type { ReactNode } from "react";

type SectionProps = {
  title: string;
  children: ReactNode;
};

export function Section({ title, children }: SectionProps) {
  return (
    <section>
      <h2>{title}</h2>
      {children}
    </section>
  );
}
```

Declare children only when supported; use semantic callback names such as `onRetry` and domain-specific props. Do not mutate props or duplicate values that can be derived while rendering. Use stable domain IDs as keys, not array indexes for changing lists.

## HeroUI and styling

Prioritize the pinned HeroUI v3 component library for every applicable frontend component need, including forms, inputs, cards, buttons, status feedback, dialogs, menus and tables. Use semantic HTML/CSS for layout/content or when no suitable library component exists; explain necessary interaction-component alternatives during review. Verify exact v3 compound component APIs and event props against the pinned release; do not copy v2 props or reimplement accessible behavior. Use native semantic elements where a library component is unnecessary. Keep wrappers thin and tied to a product need.

Use Tailwind CSS v4 utilities and centralized CSS design tokens for color, spacing, radius, typography, and light/dark themes. Prefer semantic tokens over scattered literal colors and arbitrary values. Dynamic class choices must use complete statically discoverable class strings. Keep shared class-composition helpers in one owner only when needed. Use CSS transitions first and respect `prefers-reduced-motion`.

For plain React/Vite theme switching, follow HeroUI's official v3 `useTheme` hook and root `.light`/`.dark` plus `data-theme` attributes. Do not add a second theme controller or use a framework-specific provider in Vite. HeroUI components adapt through semantic theme variables; use `dark:` variants only for one-off app-owned styling.

## Async UI and accessibility

- Model loading, empty, error, success, and background refresh separately. Keep useful existing data visible during refresh; distinguish failure from an empty library.
- Use semantic headings/landmarks, visible labels, associated validation messages, keyboard navigation, visible focus, and meaningful image alternatives. Icon-only controls need accessible names; decorative icons should be hidden from assistive technology.
- Preserve HeroUI focus management and dialog return focus. Announce meaningful progress without sending every SSE tick to a live region. Do not use color as the only status signal.
- Disable duplicate pending submissions and show an action-specific result. Destructive cleanup needs preview, ownership/conflict information, and quarantine behavior consistent with the backend.
- When implementing a supplied visual reference, align typography, proportions, spacing, illustration, gradients and shadows after applying explicit user changes. Save and inspect desktop/mobile screenshots, and correct material deviations; passing interaction tests is not evidence of visual fidelity.

Test light/dark, narrow layouts, long names, keyboard operation, and reduced motion. Avoid clickable `div` controls and raw HTML injection from media metadata.
