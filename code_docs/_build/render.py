#!/usr/bin/env python3
"""Render a code_docs walkthrough HTML source to PDF with the shared template.

Usage:  python3 render.py <body.html> <out.pdf>

The body HTML is a full <html> document (with <head><title>…</title></head>);
this injects the shared template.css (sitting next to this script) before
</head> so every walkthrough shares one stylesheet. Keep the HTML sources in
code_docs/_src/ (Go) and legacy/code_docs/_src/ (DOS) so the PDFs can be
regenerated and the wording version-controlled.
"""
import os
import sys
from weasyprint import HTML

here = os.path.dirname(os.path.abspath(__file__))
css = open(os.path.join(here, "template.css"), encoding="utf-8").read()

src, out = sys.argv[1], sys.argv[2]
html = open(src, encoding="utf-8").read()
if "</head>" in html:
    html = html.replace("</head>", f"<style>\n{css}\n</style>\n</head>", 1)
else:
    html = f"<html><head><meta charset='utf-8'><style>{css}</style></head><body>{html}</body></html>"

HTML(string=html, base_url=os.path.dirname(os.path.abspath(src))).write_pdf(out)
print(f"rendered {out}")
