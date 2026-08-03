#!/usr/bin/env python3
import html
import re
import sys

SRC = sys.argv[1]
OUT = sys.argv[2]

text = open(SRC, encoding="utf-8").read()
# Drop the very first blank padding line and any trailing newline noise minimally.
lines = text.split("\n")

SGR = re.compile(r"\x1b\[([0-9;]*)m")


def parse_runs(line):
    """Yield (text, fg, bg, bold) runs for one line."""
    runs = []
    fg = bg = None
    bold = False
    pos = 0
    buf = []

    def flush():
        if buf:
            runs.append(("".join(buf), fg, bg, bold))
            buf.clear()

    for m in SGR.finditer(line):
        buf.append(line[pos:m.start()])
        pos = m.end()
        flush()
        codes = m.group(1)
        parts = [p for p in codes.split(";") if p != ""] or ["0"]
        i = 0
        while i < len(parts):
            c = parts[i]
            if c == "0":
                fg = bg = None
                bold = False
            elif c == "1":
                bold = True
            elif c == "38" and i + 1 < len(parts) and parts[i + 1] == "2":
                fg = (parts[i + 2], parts[i + 3], parts[i + 4])
                i += 4
            elif c == "48" and i + 1 < len(parts) and parts[i + 1] == "2":
                bg = (parts[i + 2], parts[i + 3], parts[i + 4])
                i += 4
            i += 1
    buf.append(line[pos:])
    flush()
    return runs


def span(run):
    t, fg, bg, bold = run
    styles = []
    if fg:
        styles.append(f"color:rgb({fg[0]},{fg[1]},{fg[2]})")
    if bg:
        styles.append(f"background:rgb({bg[0]},{bg[1]},{bg[2]})")
    if bold:
        styles.append("font-weight:700")
    esc = html.escape(t, quote=False)
    if not styles:
        return esc
    return f'<span style="{";".join(styles)}">{esc}</span>'


body_lines = []
for line in lines:
    runs = parse_runs(line)
    body_lines.append("".join(span(r) for r in runs))
body = "\n".join(body_lines).rstrip("\n")

DOC = f"""<!doctype html>
<html><head><meta charset="utf-8"><style>
  html,body{{margin:0;background:#0d1522;}}
  #frame{{
    display:inline-block;
    background:#0B172A;
    border-radius:12px;
    padding:0 0 24px 0;
    margin:40px;
    box-shadow:0 20px 60px rgba(0,0,0,.45);
    border:1px solid #1c2c44;
  }}
  #bar{{height:44px;display:flex;align-items:center;padding:0 18px;gap:9px;}}
  #bar .d{{width:13px;height:13px;border-radius:50%;}}
  pre{{
    margin:0;padding:6px 26px 0 26px;
    font-family:Menlo,Monaco,"SF Mono","JetBrains Mono",monospace;
    font-size:15px;line-height:1.0;
    color:#D8EAFF;
    white-space:pre;
    font-variant-ligatures:none;
    -webkit-font-smoothing:antialiased;
  }}
</style></head><body>
  <div id="frame">
    <div id="bar">
      <span class="d" style="background:#ff5f57"></span>
      <span class="d" style="background:#febc2e"></span>
      <span class="d" style="background:#28c840"></span>
    </div>
    <pre>{body}</pre>
  </div>
</body></html>"""

open(OUT, "w", encoding="utf-8").write(DOC)
print("wrote", OUT)
