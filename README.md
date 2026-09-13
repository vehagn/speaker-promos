# speaker-promos

Generate speaker promo graphics for [Cloud Native Days Norway](https://cloudnativedays.no)
from the live conference program, plus draft social copy to go with them.

Posting stays manual — this tool just removes the copy-paste-retype step.

    promo list                       # index the program
    promo svg --all --out out/       # generate promo SVGs
    promo post pods-on-mars          # draft LinkedIn / Bluesky copy

Full usage is documented at the end of this file once the commands land.
