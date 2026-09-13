# speaker-promos

Generate speaker promo graphics for [Cloud Native Days Norway](https://cloudnativedays.no)
from the live conference program, plus draft social copy to go with them.

Posting stays manual — this tool removes the copy-paste-retype step, not the judgement.

```sh
go run ./cmd/promo serve                       # preview and edit everything in a browser
go run ./cmd/promo list                        # index the program
go run ./cmd/promo svg --all --out out/        # render every promo card
go run ./cmd/promo post emeka-okafor         # draft LinkedIn / Bluesky copy
```

`serve` is the one to start with: it shows every card next to its draft copy, with the
fields the tool had to guess editable in place.

The 2025 promos were drawn by hand in Inkscape: 23 near-identical 4.7 MB SVGs, each one a
manual copy-paste-retype of a photo, name, employer and talk title. 2026 has 36 talks and
49 speakers.

## Install

```sh
go build -o promo ./cmd/promo
```

Go 1.27 or newer. The only Go dependencies are `golang.org/x/image` (font metrics) and
`gopkg.in/yaml.v3` (themes and manifests). Fonts are vendored under `assets/fonts` (OFL
1.1) and HTMX under `internal/web/static` (0BSD), so the tool needs no network at runtime
beyond the conference website itself.

## Where the data comes from

There is no public API. `cnctl` talks to the website over tRPC, but every schedule and
speaker procedure is an admin procedure, and the Sanity dataset rejects anonymous reads.

What *is* public is the program page itself: `https://2026.cloudnativedays.no/program`
ships the whole schedule in its React Server Components payload, as a series of
`self.__next_f.push([1,"…"])` chunks. Concatenating those gives a flight stream whose rows
hold the conference metadata and a `schedules` array of 2 days × 3 tracks × 72 slots.
`internal/rsc` extracts and parses it, including the `$NN` back-references that long or
deduplicated strings are replaced with — without resolving those, a card would contain a
literal `$55`.

Pages are cached under `~/.cache/cnd-promos` (`--cache-ttl`, `--no-cache`), because the
program page is ~5 MB and each speaker page ~3.4 MB.

## Selectors

Every command that takes a talk accepts a `<selector>`, resolved most-specific first:

| Tier | Example |
|---|---|
| Talk id prefix | `584db4de` |
| Speaker slug | `emeka-okafor`, `ylva-sorgard` |
| Talk title substring | `"pods on mars"`, `Nok-nok-nett` |
| Speaker name substring | `"haaland"` |

The first tier that matches anything wins, so a precise identifier is never ambiguous.
Norwegian letters are folded, so `ylva-sorgard` finds `ylva-sørgard`. Use
`--all` to select the whole program.

## `promo list`

```
$ go run ./cmd/promo list
Cloud Native Days Norway 2026 — 26–27 October 2026, Bergen, Norway
36 talks, 49 speakers

DAY  TIME         TRACK               SPEAKERS                      TALK                                          SELECTOR
1    09:00–11:00  Full Day Workshops  Frøya Oliveira              Kan 🇳🇴 skyen kjøre på en brødrister?…  gunvor-rønning
1    09:00–11:00  Morning Workshops   Gudrun Nyhus and Vemund Lindved    Scaling Scheduling: the Boring Way — Lessons from Fjordstack…  astrid-sæther
…
```

`--day N` and `--speaker <slug|name>` narrow it; `--json` emits the full domain model,
abstracts included.

## `promo svg`

```sh
go run ./cmd/promo svg emeka-okafor                     # one card, portrait
go run ./cmd/promo svg --all --size both --out out/       # 72 cards
go run ./cmd/promo svg gunvor-rønning --png             # rasterise too
```

Two sizes: **portrait** 1400×2100 (as 2025) and **landscape** 1200×630 (the site's OG
ratio). `--size portrait|landscape|both`.

Cards are self-contained: the speaker photo is an embedded JPEG and the typefaces are
base64 `@font-face` rules, so one file is the whole artifact. Text is real editable
`<text>`, not outlined paths, so a card can still be adjusted by hand afterwards. A
finished portrait card is ~750 KB against 4.7 MB for the 2025 hand-drawn equivalents.

Only the faces a card actually references get embedded, and photos are requested from the
CDN as JPEG rather than the source PNG (47 KB against 600 KB at 600 px).

### Fonts

Browsers and [resvg](https://github.com/linebender/resvg) honour the embedded faces.
Inkscape and librsvg ignore base64 `@font-face` and substitute a default, so for the
edit-by-hand and PNG paths:

```sh
go run ./cmd/promo fonts install     # copy the vendored TTFs to ~/Library/Fonts
```

`--png` shells out to the first of `resvg`, `inkscape`, `rsvg-convert` or `magick` found
on PATH, at the card's own pixel size unless `--png-width` says otherwise. With none
installed it writes the SVGs anyway and tells you what to install.

### Emoji

Talk titles contain them — *"Hardening Multi-Tenancy: at Fjord Scale"*. Space
Grotesk has no emoji coverage, so runs whose glyphs are missing from the embedded font are
split into their own `<tspan>` with a system fallback family (`Apple Color Emoji`,
`Noto Color Emoji`) and measured at a 1 em approximation. Their advance is therefore
approximate and an emoji-heavy title can wrap slightly off. `promo svg` says which cards
are affected; `--strip-emoji` removes them instead.

### Themes

The built-in theme is the 2026 website brand: the house gradient `#1D4ED8` → `#06B6D4`,
Space Grotesk throughout. Everything — palette, geometry, type scale, autofit ranges — is
data:

```sh
go run ./cmd/promo theme dump > theme.yaml   # start from the built-in theme
go run ./cmd/promo svg --all --theme theme.yaml
```

A `--theme` file is merged *over* the default, so it only needs the keys it changes.
Type sizes autofit: each text style has a size range and a maximum line count, and layout
walks the range downward until a real font-metric wrap fits. When nothing fits, the text
is truncated and the card is named in a warning rather than silently clipped.

## `promo post`

```sh
go run ./cmd/promo post emeka-okafor
go run ./cmd/promo post --platform bluesky --all
```

```
── bluesky (256/300 chars) ──

Lucia Ferreira (Havbris) is speaking at Cloud Native Days Norway 2026 🎤

“Nettverket er ikke dødt — det er bare ikke der du la det”

09:10–09:50 · Tuesday 27 October

@audun.bsky.social
https://2026.cloudnativedays.no/program

check before posting:
  - employer for Lucia Ferreira guessed as "Havbris" from "Senior Cloud Dev Advocate @Havbris" — check it
```

Bluesky copy is assembled to fit 300 characters with the handles and link reserved first,
so shortening never eats the link. LinkedIn copy is longer and lists profile URLs
separately, because LinkedIn only turns a mention into a link when it is picked from its
own autocomplete.

### The link goes to the program

Posts link to `/program`, not to a speaker profile. Talks have no page of their own: the
site's sitemap carries 49 `/speaker/<slug>` URLs and exactly one `/program`, and the
program page keeps its filters in client state with no URL parameters and no per-talk
anchors — so there is nothing talk-specific to link to, and the program is the closest
thing to "this talk".

Speakers are still surfaced: as `@handle` mentions in the Bluesky body, and as profile URLs
under *profiles to mention* for LinkedIn.

### Employers are guessed

There is no structured employer field. `speaker.title` is free text and inconsistent:
`"Staff Developer Advocate at Vestbit Labs"`, `"Senior Cloud Dev Advocate @Havbris"`,
`"Utvikler hos Bergsdal"`, `"Bysten Labs"`, `""`. `post` splits on the separators that actually
occur and treats a title with no separator as a bare employer — then **marks every guess
for checking**, as above.

Social handles for @-mentions are scraped from `/speaker/<slug>`, which exposes LinkedIn,
Bluesky, GitHub and X links where a speaker set them. This is the most fragile part of the
tool and is only used for optional mention suggestions; `--no-links` skips it.

Corrections go in a manifest, described below.

## `promo serve`

```sh
go run ./cmd/promo serve                    # http://localhost:8787
go run ./cmd/promo serve --size landscape --addr :9000
```

Every talk as a row: the real card on the left, and on the right its slot, the editable
speaker facts, and both drafts with live character counts. The Bluesky count has a meter
because 300 characters is the binding constraint. Guessed employers are outlined in amber
with the text they were guessed from.

Editing a field saves it to the manifest immediately and re-renders that one card — there
is no save button, and nothing to lose if the browser closes. `git diff promos.yaml` is
the record; `git checkout promos.yaml` is the undo.

- **Display title** shortens a title on the card without touching the program.
- **hidden** excludes a talk from `--all` and from the Export button.
- **Export all** writes every visible card to `--out`, the same as `promo svg --all`.

Startup fetches the 49 speaker profile pages once (~3 MB each, then cached on disk); after
that page loads are instant. `--no-links` skips it entirely. Cards are served as the same
self-contained SVGs you would post, so the preview is the artifact rather than an
approximation — which does mean a fully scrolled page pulls ~24 MB from localhost.

## Overrides

`svg`, `post` and `serve` all read `promos.yaml` (`--manifest`), a multi-document
Kubernetes-style manifest. It is meant to be committed: it is the record of every
correction made to data the tool guessed.

```yaml
apiVersion: promo.cloudnativedays.no/v1alpha1
kind: SpeakerOverride
metadata:
  name: gunvor-rønning          # speaker slug, as printed by `promo list`
spec:
  employer: Bysten Labs
  job: Infrastructure Engineer
  links:
    linkedin: https://www.linkedin.com/in/dario
    bluesky: dario.bsky.social
---
apiVersion: promo.cloudnativedays.no/v1alpha1
kind: TalkOverride
metadata:
  name: 584db4de-d0ac-4d3a-9fc7-33d541b6c862   # talk id
spec:
  displayTitle: Kort tittel
  hidden: false
```

An overridden employer stops being reported as a guess, and reaches the card's role line as
well as the copy — the card saying the wrong thing is usually why you are correcting it.

`apiVersion`, `kind` and every field name are validated with the line number, so a typo is
an error rather than an override that silently does nothing:

```
promo: promos.yaml: line 6: unknown field "employeer" (known fields: employer, job, links)
```

`promo serve` writes this file sorted by kind then name, so it stays diff-stable, and
removes an object once all of its fields are cleared.

## Layout

```
cmd/promo/          subcommands over stdlib flag
internal/rsc/       flight extraction, row table, $ref resolution
internal/cnd/       fetch + domain model; portable text; speaker-page links
internal/cache/     on-disk HTTP cache
internal/theme/     theme structs, YAML loading, embedded default-2026
internal/layout/    font metrics, greedy wrap, size autofit
internal/render/    SVG emitters (portrait, landscape)
internal/post/      LinkedIn / Bluesky copy
internal/manifest/  override manifests (load, validate, save)
internal/web/       preview server, templates, vendored HTMX
assets/fonts/       vendored OFL fonts + licences
```

## Tests

```sh
go test ./...
```

RSC parsing runs against a committed program-page fixture (including a `$ref` and a
multi-byte length-prefixed row), and the cards against golden SVGs — `go test ./... -update`
rewrites those. Nothing in the test suite touches the network.
