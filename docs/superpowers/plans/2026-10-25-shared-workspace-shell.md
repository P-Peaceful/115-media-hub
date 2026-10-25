# Shared Workspace Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the authenticated app's shared sidebar and top bar with the supplied 115 Media Hub screenshot.

**Architecture:** Keep the frame in `AppLayout` and preserve its `Outlet`. Use the existing icon and HeroUI packages; sidebar entries remain presentational, with root mapped to Overview.

**Tech Stack:** React, TypeScript, Tailwind CSS 4, HeroUI 3, lucide-react.

**Spec:** `docs/superpowers/specs/2026-10-25-shared-workspace-shell-design.md`

## Global Constraints

- Do not alter authentication, backend APIs, routes, or page content.
- Keep existing sign-out and theme switching behavior.
- Do not add dependencies or fabricate navigation destinations.

## Review Focus

- Narrow screens: sidebar and main area remain usable without horizontal overflow.
- Existing authentication actions: logout and theme controls remain available and accessible.
- Nested and root routes: breadcrumb and active nav label remain consistent with the route.

---

### Task 1: Update the shared authenticated frame

**Files:**
- Modify: `frontend/src/app/layouts/AppLayout.tsx`

**Interfaces:**
- Consumes: existing `Outlet`, `useAuth`, and `usePreferences` interfaces.
- Produces: shared shell rendering sidebar, top bar, and the unchanged route outlet.

- [x] Implement the reference shell: fixed-width white sidebar with brand, workspace subtitle, eight nav labels, bottom Settings item and connection tile; top bar with breadcrumb, task status, theme control and avatar.
- [x] Keep navigation presentational, derive root active state as Overview, and keep `Outlet` content unchanged.
- [x] Run frontend typecheck, lint, and build.
- [ ] Visually inspect the authenticated frame at desktop and narrow viewport widths. The browser opens `/login` without an existing session, so the protected shell could not be rendered for comparison.
