# speaker-promos

Generate speaker promo graphics for [Cloud Native Days Norway](https://cloudnativedays.no)
from the live conference program, plus draft social copy to go with them.

Posting stays manual — this tool removes the copy-paste-retype step, not the judgement.

```sh
go run ./cmd/promo serve                       # preview and edit everything in a browser
go run ./cmd/promo list                        # index the program
go run ./cmd/promo export --all --out out/     # a folder per talk: cards, copy, config
go run ./cmd/promo post dario-haaland          # draft LinkedIn / Bluesky copy
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
| Speaker slug | `dario-haaland`, `audun-oygard` |
| Talk title substring | `"nok nett"`, `nok-nett` |
| Speaker name substring | `"haaland"` |

The first tier that matches anything wins, so a precise identifier is never ambiguous.
Norwegian letters are folded, so `audun-oygard` finds `audun-øygard`. Use
`--all` to select the whole program.

## `promo list`

```
$ go run ./cmd/promo list
Cloud Native Days Norway 2026 — 26–27 October 2026, Bergen, Norway
36 talks, 49 speakers

DAY  TIME         TRACK               SPEAKERS                      TALK                                          SELECTOR
1    09:00–11:00  Full Day Workshops  Dario Haaland                 Kan 🇳🇴 skyen kjøre på en brødrister?      dario-haaland
1    09:00–11:00  Morning Workshops   Espen Tveitan and Leif Grim…  Scaling Scheduling: the Boring Way — Lesson…  espen-tveitan
…
```

`--day N` and `--speaker <slug|name>` narrow it; `--json` emits the full domain model,
abstracts included.

## `promo export`

```sh
go run ./cmd/promo export dario-haaland                     # one talk's folder
go run ./cmd/promo export --all --size both --out out/      # the whole program
go run ./cmd/promo export --all --formats svg               # skip rasterising
go run ./cmd/promo export --all --width 1000                # smaller PNG/JPEG
```

One folder per talk, named so a directory listing falls into schedule order:

```
out/d1-0900-dario-haaland-kan-skyen-kjore-pa-en-brodrister/
├── portrait.svg      1400×2100, the editable original
├── portrait.png      rasterised at the card's own size
├── portrait.jpg      the same, re-encoded (q88 by default)
├── landscape.svg     1200×630, the site's OG ratio
├── landscape.png
├── landscape.jpg
├── linkedin.txt      the post body, nothing else — paste it verbatim
├── bluesky.txt       likewise, inside the 300-character limit
├── NOTES.txt         what to check first, and the profiles to mention
└── promo.yaml        the overrides that produced all of the above
```

`--formats` picks any of `svg,png,jpg` (all three by default) and `--size` any of
`portrait,landscape,both`. The review notes are a separate file from the copy deliberately:
`linkedin.txt` and `bluesky.txt` hold the post and nothing else, so nothing about guessed
employers can be pasted into a real post by accident.

`promo.yaml` is two things in one file. A `TalkInfo` object records everything the tool knew
when it produced the folder — the conference, the slot, track, format, level, topics,
abstract, and each speaker's resolved employer, handles, profile URL and whether the card
got a real photo or fell back to initials. Below it sit the editable `SpeakerOverride` and
`TalkOverride` objects, pre-filled with the values the cards actually used: the correction
where you made one, the guess otherwise.

`TalkInfo` is ignored when the file is loaded, so a bundle can be passed straight back with
`--manifest` after editing — no need to strip the record first. There is deliberately no
generation timestamp, so re-exporting produces an identical folder.

Cards are self-contained: the speaker photo is an embedded JPEG and the typefaces are
base64 `@font-face` rules, so one SVG is the whole artifact. Text is real editable
`<text>`, not outlined paths, so a card can still be adjusted by hand afterwards. A
finished portrait card is ~750 KB against 4.7 MB for the 2025 hand-drawn equivalents.

Only the faces a card actually references get embedded, and photos are requested from the
CDN as JPEG rather than the source PNG (47 KB against 600 KB at 600 px).

The full program at both sizes is 36 folders and 360 files, around 170 MB — nearly all of
it PNG, which is 2.3 MB per portrait card at full size. `--width` trades resolution for
size, and `--formats svg,jpg` skips PNG entirely.

### Fonts

Browsers and [resvg](https://github.com/linebender/resvg) honour the embedded faces.
Inkscape and librsvg ignore base64 `@font-face` and substitute a default, so for the
edit-by-hand and PNG paths:

```sh
go run ./cmd/promo fonts install     # copy the vendored TTFs to ~/Library/Fonts
```

PNG comes from the first of `resvg`, `inkscape`, `rsvg-convert` or `magick` found on PATH.
JPEG is then re-encoded from that PNG with the standard library rather than asked of the
tool, because only one of the four can write JPEG at all — so JPEG is available whenever
PNG is. With no rasteriser installed, the SVGs, copy and manifests are still written and
the run says what to install.

### Emoji

Talk titles contain them — *"Kan 🇳🇴 skyen kjøre på en brødrister?"*. Space
Grotesk has no emoji coverage, so runs whose glyphs are missing from the embedded font are
split into their own `<tspan>` with a system fallback family (`Apple Color Emoji`,
`Noto Color Emoji`) and measured at a 1 em approximation. Their advance is therefore
approximate and an emoji-heavy title can wrap slightly off. `promo export` says which cards
are affected; `--strip-emoji` removes them instead.

### Themes

The built-in theme is the 2026 website brand: the house gradient `#1D4ED8` → `#06B6D4`,
Space Grotesk throughout. Everything — palette, geometry, type scale, autofit ranges — is
data:

```sh
go run ./cmd/promo theme dump > theme.yaml   # start from the built-in theme
go run ./cmd/promo export --all --theme theme.yaml
```

A `--theme` file is merged *over* the default, so it only needs the keys it changes.
Type sizes autofit: each text style has a size range and a maximum line count, and layout
walks the range downward until a real font-metric wrap fits. When nothing fits, the text
is truncated and the card is named in a warning rather than silently clipped.

## `promo post`

```sh
go run ./cmd/promo post dario-haaland
go run ./cmd/promo post --platform bluesky --all
go run ./cmd/promo post --all --language no          # force Norwegian
```

```
── bluesky (259/300 chars) ──

Dario Haaland (Bysten Labs) is running a workshop at Cloud Native Days Norway 2026 🎤

“Kan 🇳🇴 skyen kjøre på en brødrister?”

Plattformer bygges best når teamet forstår hele stacken.

09:00–11:00 · Monday 26 October
https://2026.cloudnativedays.no/program

check before posting:
  - employer for Dario Haaland guessed as "Bysten Labs" from "Bysten Labs" — check it
```

Bluesky copy is assembled to fit 300 characters with the handles and link reserved first,
so shortening never eats the link. LinkedIn copy is longer and lists profile URLs
separately, because LinkedIn only turns a mention into a link when it is picked from its
own autocomplete.

### Norwegian and English

The program is bilingual, and an English sentence introducing a Norwegian abstract reads
badly. The copy is written in the talk's own language:

```
Dario Haaland (Bysten Labs) holder workshop på Cloud Native Days Norway 2026 🎤

«Kan 🇳🇴 skyen kjøre på en brødrister?»

Plattformer bygges best når teamet forstår hele stacken.

09:00–11:00 · mandag 26. oktober
```

Language is detected per talk by counting function words that occur in one language and
not the other — not by looking for æ, ø and å, which are a strong signal when present but
missing from plenty of Norwegian text. The abstract dominates the title by being longer,
which is what you want for a bilingual title like *"Friheten i Koden: Digital Sovereignty
for the Nordic Age"*: the copy follows the language the body is actually written in. Across
the 2026 program this reads 8 talks as Norwegian and 28 as English.

Detection is only a default. `--language en|no` forces one for a run, and `language:` on a
`TalkOverride` forces one for a single talk, which wins over the flag. `promo serve` has a
per-talk selector whose *auto* option names what detection picked, and each draft carries a
badge showing the language it came out in.

What changes is the wording the tool writes: the verb phrase, the conjunction, the
quotation marks (`«»` rather than `“”`), and the weekday and month names, which are looked
up rather than taken from `time.Format` — that only knows English. The "check before
posting" notes stay in English; they are for you, not for the post.

**The cards stay in English.** Their text is conference chrome — the date line, the slot,
the format — and it is identical on every card, so mixing languages across a set of images
would look like a mistake rather than a choice. A post is different because it quotes the
abstract.

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
`"Senior Platform Engineer at Vestbit"`, `"Senior Consultant @Nordvik"`,
`"Utvikler hos Bergsdal Consulting"`, `"Bysten Labs"`, `""`. `post` splits on the separators that actually
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

- **Photo** gives a speaker a picture when they have none, or replaces a poor one.
- **Copy language** overrides the detected language; *auto* names what it detected.
- **Display title** shortens a title on the card without touching the program.
- **hidden** excludes a talk from `--all` and from the Export button.
- **Export all** writes a bundle per visible talk to `--out`, through the same code as
  `promo export --all`, so the two produce identical folders.

Startup fetches the 49 speaker profile pages once (~3 MB each, then cached on disk); after
that page loads are instant. `--no-links` skips it entirely. Cards are served as the same
self-contained SVGs you would post, so the preview is the artifact rather than an
approximation — which does mean a fully scrolled page pulls ~24 MB from localhost.

## Overrides

`export`, `post` and `serve` all read `promos.yaml` (`--manifest`), a multi-document
Kubernetes-style manifest. It is meant to be committed: it is the record of every
correction made to data the tool guessed.

```yaml
apiVersion: promo.cloudnativedays.no/v1alpha1
kind: SpeakerOverride
metadata:
  name: dario-haaland             # speaker slug, as printed by `promo list`
spec:
  employer: Bysten Labs
  job: Infrastructure Engineer
  image: photos/dario.jpg         # URL, or a path beside this manifest
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
  language: no                                 # en or no; omit to auto-detect
```

An overridden employer stops being reported as a guess, and reaches the card's role line as
well as the copy — the card saying the wrong thing is usually why you are correcting it.

### Photos

Five of the 2026 speakers have no photo and render a monogram of their initials instead,
and a CMS photo is sometimes simply bad. `image:` replaces it with either a URL or a file
on disk; a relative path resolves against the manifest's own directory, so a bundle can
carry its own photo next to the `promo.yaml` that names it.

Transform parameters (crop, size, JPEG re-encode) are only added for the CMS CDN, which is
the only host that understands them — an overridden photo is fetched exactly as given. The
card clips it to the rounded square either way, so an off-square photo is cropped rather
than squashed.

`promo serve` shows a **Photo** field per speaker, outlined in cyan and labelled *none,
showing initials* when the card had to fall back. `hasPhoto` in each bundle's `TalkInfo`
says the same thing for a whole export — and it reports whether the photo was actually
*fetched*, so a URL that 404s shows up as `false` rather than looking configured.

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
internal/raster/    SVG → PNG via an external tool, PNG → JPEG via stdlib
internal/export/    per-talk bundles, shared by the CLI and the server
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
