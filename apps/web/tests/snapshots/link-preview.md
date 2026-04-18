# link-preview snapshot

Expected unfurl behavior when a learner pastes one of these URLs into Slack,
iMessage, LinkedIn, or WhatsApp. Verified manually during `web/proof` review.

## `/:lang/p/:uuid`
- **og:title**: `<name> . aprendiendo Claude Code` (es) / `<name> . learning Claude Code` (en)
- **og:description**: voice `share.body_profile` with `{streak}` interpolated
- **og:image**: `/og/profile/:uuid` . 1200x630 . burgundy substrate, warm cream wordmark, flame dot, XP value pinned bottom-right
- **twitter:card**: `summary_large_image`

Confirm on Slack (free tier) by pasting `https://learn.example.com/es/p/00000000-0000-4000-8000-000000000001` into a DM and waiting 2s.

## `/:lang/p/:uuid/m/:lessonId`
- **og:title**: module title
- **og:description**: voice `share.body_module` with `{moduleTitle}` interpolated
- **og:image**: `/og/module/:uuid/:lessonId` . 1200x630 . "MODULO TERMINADO" eyebrow, module title, date pinned bottom-right

## `/:lang/certificate/:id`
- **og:title**: `<name> . learn.example.com`
- **og:description**: voice `share.body_certificate`
- **og:image**: `/og/certificate/:id` . 1200x630 . "CERTIFICADO" eyebrow, name, "Fundamentos de Claude Code" subtitle, module count pinned bottom-right

## `/verify/:id`
Intentionally not sharable as an unfurl: no og:image configured beyond the
site-wide fallback. The page is for trust, not for marketing.

## local repro

```
CONTROL_PLANE_URL=http://127.0.0.1:1 pnpm --filter web start
curl -so /tmp/og.png http://localhost:3000/og/profile/00000000-0000-4000-8000-000000000001
file /tmp/og.png  # PNG image data, 1200 x 630
```
