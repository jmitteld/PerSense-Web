#!/usr/bin/env python3
"""Build the PerSense-Web manual test plan PDF from the three section markdown files."""
import re
from reportlab.lib.pagesizes import letter, landscape
from reportlab.lib.units import inch
from reportlab.lib import colors
from reportlab.platypus import (BaseDocTemplate, PageTemplate, Frame, Paragraph,
                                Spacer, Table, TableStyle, PageBreak, NextPageTemplate)
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.enums import TA_LEFT, TA_CENTER

PAGE = landscape(letter)
M = 0.35 * inch
USABLE = PAGE[0] - 2 * M  # 741.6

ACCENT = colors.HexColor('#1f3a5f')
BAND = colors.HexColor('#dce6f1')
ALT = colors.HexColor('#f4f7fb')
GRID = colors.HexColor('#b8c4d4')

styles = getSampleStyleSheet()
cell = ParagraphStyle('cell', parent=styles['Normal'], fontName='Helvetica',
                      fontSize=6.6, leading=8.2, spaceBefore=0, spaceAfter=0)
cell_id = ParagraphStyle('cellid', parent=cell, fontName='Helvetica-Bold')
cell_hdr = ParagraphStyle('cellhdr', parent=cell, fontName='Helvetica-Bold',
                          fontSize=7.2, textColor=colors.white)
cell_band = ParagraphStyle('cellband', parent=cell, fontName='Helvetica-Bold',
                           fontSize=7.4, textColor=ACCENT)
h1 = ParagraphStyle('h1x', parent=styles['Title'], fontSize=22, leading=27, textColor=ACCENT)
h2 = ParagraphStyle('h2x', parent=styles['Heading1'], fontSize=15, leading=19, textColor=ACCENT,
                    spaceBefore=0, spaceAfter=6)
body = ParagraphStyle('bodyx', parent=styles['Normal'], fontSize=8.6, leading=11.6,
                      spaceAfter=6)
small = ParagraphStyle('smallx', parent=styles['Normal'], fontSize=8, leading=10.5,
                       spaceAfter=4)


def md_to_rl(text):
    """Escape + convert minimal markdown (bold, code, arrows) to reportlab markup."""
    t = text.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;')
    t = re.sub(r'\*\*(.+?)\*\*', r'<b>\1</b>', t)
    t = re.sub(r'\*(.+?)\*', r'<i>\1</i>', t)
    t = re.sub(r'`(.+?)`', r'<font face="Courier" size="6.2">\1</font>', t)
    return t


def parse_md(path):
    intro, rows = [], []
    with open(path) as f:
        lines = f.read().splitlines()
    in_table = False
    for ln in lines:
        s = ln.strip()
        if s.startswith('|'):
            in_table = True
            cells = [c.strip() for c in s.strip('|').split('|')]
            # pad/trim to 5
            cells = (cells + [''] * 5)[:5]
            if set(cells[0]) <= set('-: ') and cells[0]:
                continue  # separator row
            if cells[0] in ('ID',):
                continue  # header row
            rows.append(cells)
        elif not in_table:
            if s.startswith('# '):
                continue
            if s:
                intro.append(s)
    return ' '.join(intro), rows


def build_section_table(rows, prefix):
    col_w = [46, 88, 205, 232, 92, 30, 48.6]
    assert abs(sum(col_w) - USABLE) < 1.5, sum(col_w)
    data = [[Paragraph('ID', cell_hdr), Paragraph('Title / Scenario', cell_hdr),
             Paragraph('Inputs', cell_hdr), Paragraph('Expected / Verify', cell_hdr),
             Paragraph('Covers (code path)', cell_hdr), Paragraph('P / F', cell_hdr),
             Paragraph('Notes', cell_hdr)]]
    band_rows, data_rows = [], 0
    for cells in rows:
        if not re.match(rf'^{prefix}-\d+$', cells[0]):
            # group band
            data.append([Paragraph(md_to_rl(cells[1]), cell_band), '', '', '', '', '', ''])
            band_rows.append(len(data) - 1)
        else:
            data_rows += 1
            data.append([Paragraph(md_to_rl(cells[0]), cell_id),
                         Paragraph(md_to_rl(cells[1]), cell),
                         Paragraph(md_to_rl(cells[2]), cell),
                         Paragraph(md_to_rl(cells[3]), cell),
                         Paragraph(md_to_rl(cells[4]), cell),
                         '', ''])
    t = Table(data, colWidths=col_w, repeatRows=1, splitByRow=1)
    style = [
        ('BACKGROUND', (0, 0), (-1, 0), ACCENT),
        ('GRID', (0, 0), (-1, -1), 0.4, GRID),
        ('VALIGN', (0, 0), (-1, -1), 'TOP'),
        ('LEFTPADDING', (0, 0), (-1, -1), 3),
        ('RIGHTPADDING', (0, 0), (-1, -1), 3),
        ('TOPPADDING', (0, 0), (-1, -1), 2.5),
        ('BOTTOMPADDING', (0, 0), (-1, -1), 2.5),
    ]
    stripe = True
    for i in range(1, len(data)):
        if i in band_rows:
            style.append(('BACKGROUND', (0, i), (-1, i), BAND))
            style.append(('SPAN', (0, i), (-1, i)))
            stripe = True
        else:
            if stripe:
                pass
            if (i - max([b for b in band_rows if b < i] or [0])) % 2 == 0:
                style.append(('BACKGROUND', (0, i), (-1, i), ALT))
    t.setStyle(TableStyle(style))
    return t, data_rows


def footer(canvas, doc):
    canvas.saveState()
    canvas.setFont('Helvetica', 7)
    canvas.setFillColor(colors.HexColor('#666666'))
    canvas.drawString(M, 0.22 * inch, 'PerSense-Web Manual Test Plan — v1.0 — 2026-07-22')
    canvas.drawRightString(PAGE[0] - M, 0.22 * inch, f'Page {doc.page}')
    canvas.restoreState()


doc = BaseDocTemplate('/home/user/persense-testplan/PerSense_Manual_Test_Plan.pdf',
                      pagesize=PAGE, leftMargin=M, rightMargin=M,
                      topMargin=0.4 * inch, bottomMargin=0.42 * inch,
                      title='PerSense-Web Manual Test Plan',
                      author='PerSense-Web project')
frame = Frame(M, 0.42 * inch, USABLE, PAGE[1] - 0.82 * inch, id='f')
doc.addPageTemplates([PageTemplate(id='p', frames=[frame], onPage=footer)])

story = []

# ---------- Cover / instructions ----------
story.append(Spacer(1, 30))
story.append(Paragraph('Per%Sense Web Port — Manual Test Plan', h1))
story.append(Paragraph('300 side-by-side test cases vs the legacy DOS application '
                       '(100 Amortization · 100 Mortgage · 100 Present Value)',
                       ParagraphStyle('sub', parent=body, fontSize=11, leading=14,
                                      textColor=colors.HexColor('#44546a'))))
story.append(Spacer(1, 10))
cover_paras = [
    ('Purpose', 'This document is a manual acceptance test plan for the Go/web port of the '
     'Per%Sense DOS financial application. Each case gives concrete inputs to type into the '
     'web app, the behavior to verify, and the engine code path it exercises. The '
     '<b>DOS application is the financial authority</b>: unless a case says otherwise, '
     '&quot;matches DOS&quot; means running the identical inputs in the legacy DOS program '
     'side-by-side and confirming agreement to the cent (money) or displayed precision '
     '(rates/APRs). Do not compare against the Windows version or its help examples — '
     'several documented cases exist where the Windows help differs from actual DOS behavior.'),
    ('Setup', 'Run the PerSense-Web binary and open it in a browser; run the DOS Per%Sense '
     'in DOSBox (or equivalent) beside it. Before every case, Clear All in the web app, reset '
     'the DOS worksheet, and set the computational settings identically in both programs. '
     'Dates are typed MM/DD/YYYY. Each section’s intro (first page of the section) defines its '
     'default settings and any input shorthand used by that section’s cases.'),
    ('Recording results', 'Mark P (pass) or F (fail) in the P/F column and use Notes for the '
     'observed value or delta. For any failure, record BOTH the web output and the DOS output '
     'for the same inputs — a divergence can only be adjudicated against DOS. Cases flagged '
     '<b>DELIBERATE DIVERGENCE</b> or <b>[deliberate difference]</b> document places where the '
     'port intentionally does not reproduce a DOS bug or limitation (with the project doc that '
     'records the decision); do not file those as defects, but do record the DOS output in Notes.'),
    ('Coverage', 'Each section’s 100 cases are grouped: forward calculations across bases, '
     'frequencies and methods; backward (field-presence) solves for each solvable unknown; '
     'advanced options and combinations; errors, refusals and boundary values; and UI/workflow '
     'behaviors (green solved cells, hardening, undo, auto-calculate, CSV export). The Covers '
     'column names the Go engine function or branch each case exercises, so a failure can be '
     'routed straight to the responsible code.'),
    ('Suggested order', 'Run each group’s smoke cases first (the first few cases of each group '
     'are the simplest), then proceed to combinations. Groups are independent — sections can be '
     'split across testers, but keep any case that references an earlier case (e.g. '
     '&quot;the solved payment P from AMZ-044&quot;) with the same tester.'),
]
for title, text in cover_paras:
    story.append(Paragraph(f'<b>{title}.</b> {text}', body))
story.append(Spacer(1, 6))
sum_tbl = Table([
    ['Section', 'Cases', 'ID range', 'Screen'],
    ['Amortization', '100', 'AMZ-001 … AMZ-100', 'Amortization (schedule, solvers, advanced options, payoff)'],
    ['Mortgage', '100', 'MTG-001 … MTG-100', 'Mortgage worksheet (12-row grid, APR compare, What-If)'],
    ['Present Value', '100', 'PV-001 … PV-100', 'Present Value (lump/periodic grids, COLA, VR, actuarial)'],
], colWidths=[110, 50, 130, 380])
sum_tbl.setStyle(TableStyle([
    ('BACKGROUND', (0, 0), (-1, 0), ACCENT),
    ('TEXTCOLOR', (0, 0), (-1, 0), colors.white),
    ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
    ('FONTNAME', (0, 1), (-1, -1), 'Helvetica'),
    ('FONTSIZE', (0, 0), (-1, -1), 8.5),
    ('GRID', (0, 0), (-1, -1), 0.4, GRID),
    ('ROWBACKGROUNDS', (0, 1), (-1, -1), [colors.white, ALT]),
    ('LEFTPADDING', (0, 0), (-1, -1), 5),
    ('TOPPADDING', (0, 0), (-1, -1), 3.5),
    ('BOTTOMPADDING', (0, 0), (-1, -1), 3.5),
]))
story.append(sum_tbl)
story.append(PageBreak())

# ---------- Sections ----------
sections = [
    ('Section 1 — Amortization (AMZ-001 … AMZ-100)', 'amz.md', 'AMZ'),
    ('Section 2 — Mortgage (MTG-001 … MTG-100)', 'mtg.md', 'MTG'),
    ('Section 3 — Present Value (PV-001 … PV-100)', 'pv.md', 'PV'),
]
counts = {}
for title, fname, prefix in sections:
    intro, rows = parse_md(f'/home/user/persense-testplan/{fname}')
    story.append(Paragraph(title, h2))
    story.append(Paragraph(md_to_rl(intro), small))
    story.append(Spacer(1, 4))
    tbl, n = build_section_table(rows, prefix)
    counts[prefix] = n
    story.append(tbl)
    if fname != sections[-1][1]:
        story.append(PageBreak())

doc.build(story)
print('counts:', counts)
