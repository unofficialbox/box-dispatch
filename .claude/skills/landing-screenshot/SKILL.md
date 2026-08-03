---
name: landing-screenshot
description: Regenerate the README hero image (docs/landing.png) — a color screenshot of the box-dispatch welcome screen with the real punk-rock 🤘 emoji. Use when the landing UI changed or the README screenshot needs refreshing.
---

# landing-screenshot

Produces `docs/landing.png`, the colored welcome-screen image in the README. This is **not**
a plain terminal screenshot — two hard constraints forced a custom pipeline:

1. **lipgloss strips ANSI color when stdout is not a TTY.** Rendering `View()` from a test or
   pipe yields monochrome unless you force the color profile first.
2. **`freeze` (resvg) cannot render Apple Color Emoji** — the 🤘 comes out as a "tofu" box.
   So we render through a real browser instead of an SVG rasterizer.

Pipeline: **capture colored ANSI → convert to HTML → screenshot with headless Chrome.**
`ansi2html.py` in this skill directory does the middle step.

## Step 1 — capture the welcome screen as truecolor ANSI

`newDispatchShell()` and `View()` are unexported (`package main`), so capture from a
throwaway test in `cmd/box-dispatch/`. Force the truecolor profile **before** calling
`View()`, and feed a window size so layout fills out. Create
`cmd/box-dispatch/zz_landing_dump_test.go`:

```go
package main

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Throwaway: dumps the welcome screen as truecolor ANSI to landing.ans.
// Delete after generating the screenshot — do not commit.
func TestDumpLanding(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor) // else color is stripped off-TTY
	m := newDispatchShell()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 118, Height: 40})
	if err := os.WriteFile("landing.ans", []byte(updated.View()), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

```bash
cd cmd/box-dispatch && go test -run TestDumpLanding . && mv landing.ans /tmp/landing.ans
rm cmd/box-dispatch/zz_landing_dump_test.go   # capture files/tests are never committed
```

Tune `Width`/`Height` to frame the welcome screen (the original used ~118×40). If the punk
emoji or a menu row is clipped, adjust and re-run.

## Step 2 — ANSI → HTML

```bash
python3 .claude/skills/landing-screenshot/ansi2html.py /tmp/landing.ans /tmp/landing.html
```

`ansi2html.py` parses SGR codes (reset/bold/`38;2` fg/`48;2` bg truecolor), wraps the output
in a dark terminal-chrome card, and — critically — sets `line-height:1.0` so block-art rows
tile with no vertical gaps. Emoji render as real glyphs because it's just HTML text.

## Step 3 — HTML → PNG with headless Chrome (renders color emoji)

```bash
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
"$CHROME" --headless=new --disable-gpu --hide-scrollbars \
  --force-device-scale-factor=2 --window-size=1200,820 \
  --screenshot=/tmp/landing_hi.png "file:///tmp/landing.html"
```

- `--force-device-scale-factor=2` gives a crisp 2× (retina) image.
- Size the `--window-size` to the card; crop/trim afterward if needed.
- The **Chrome MCP browser blocks `file://`** — if you drive it via the extension instead of
  headless CLI, serve the file first: `python3 -m http.server 8791` and open
  `http://localhost:8791/landing.html`.

## Step 4 — install and wire into the README

```bash
cp /tmp/landing_hi.png docs/landing.png
```

The README already references it (after the `# box-dispatch` heading):

```html
<p align="center">
  <img src="docs/landing.png" alt="box-dispatch interactive launch shell — the welcome screen ..." width="900">
</p>
```

Verify the 🤘 is a full-color emoji (not a mono box) before committing. Commit only
`docs/landing.png` (and README if the block changed) — never the throwaway test or the
`/tmp` intermediates.
