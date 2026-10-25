# Shared workspace shell design

## Scope

Update the authenticated shared application frame to match the supplied 115 Media Hub reference: a fixed left sidebar, a compact top bar, and a light gray workspace background. Keep the existing routed page content rendered through `Outlet`.

## Layout

- Sidebar: brand mark and name, workspace subtitle, navigation labels `总览`, `网盘浏览`, `媒体库`, `STRM 管理`, `任务中心`, `海报墙`, `刮削回传`, and `运行日志`, a bottom `设置` item, and a connection status card.
- Top bar: current workspace breadcrumb, running-task status, theme control, and a compact user avatar. Keep unavailable task status neutral rather than inventing a count.
- Main area: preserve the existing page component and its content without introducing wizard/path-detection UI.

## Behavior boundaries

- Keep existing sign-out and theme switching behavior.
- Navigation is presentational for now; do not invent destination routes or page functionality.
- Connection status remains `待授权` because the 115 connection state is not implemented in this shell.
- Mark the current item from the existing route where practical; the root page maps to Overview.
- Do not change authentication, backend APIs, or other routes.

## Visual direction

Use the supplied screenshot as the reference for proportions, spacing, colors, icon alignment, and sidebar placement. Use existing project components and icon dependencies; add no dependencies.
