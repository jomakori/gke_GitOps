#!/usr/bin/env python3
"""Parse an IRS Wage & Income transcript (PDF or raw text) into structured JSON.

Usage:
    uv run --with pypdf parse_wi_transcript.py transcript.pdf [-o transcripts.json]
    parse_wi_transcript.py transcript.txt              # raw text fallback

Output shape:
{
  "year_hint": "2025",
  "w2s":  [{"employer": ..., "ein": ..., "wages": ..., "fed_withheld": ...,
            "ss_wages": ..., "ss_withheld": ..., "medicare_wages": ...,
            "medicare_withheld": ..., "state": ..., "state_withheld": ...}],
  "1099s": [{"type": "INT", "payer": ..., "tin": ..., "amounts": {...}}]
}

Notes:
- W&I transcripts have a consistent text layout; this is regex-driven on
  extracted text. Section headers like "WAGE AND TAX STATEMENT" (W-2),
  "INTEREST INCOME" (1099-INT), "DIVIDENDS AND DISTRIBUTIONS" (1099-DIV),
  "NONEMPLOYEE COMPENSATION" (1099-NEC), "PAYMENTS IN LIEU OF DIVIDENDS",
  "UNEMPLOYMENT COMPENSATION" (1099-G), "SOCIAL SECURITY BENEFIT" (1099-SSA)
  split the document into records.
- PDFs with a "This is a copy of your information" cover page are skipped.
- Re-verify extracted numbers against the PDF before e-filing.
"""
import json
import re
import sys

try:
    from pypdf import PdfReader
except ImportError:
    PdfReader = None

DOC_HEADERS = [
    ("W-2", "WAGE AND TAX STATEMENT"),
    ("SSA", "SOCIAL SECURITY BENEFIT STATEMENT"),
    ("INT", "INTEREST INCOME"),
    ("DIV", "DIVIDENDS AND DISTRIBUTIONS"),
    ("NEC", "NONEMPLOYEE COMPENSATION"),
    ("MISC", "MISCELLANEOUS INCOME"),
    ("G", "UNEMPLOYMENT COMPENSATION"),
    ("R", "PENSION OR ANNUITY"),
    ("B", "PROCEEDS FROM BROKER"),
    ("K", "PARTNER'S SHARE OF INCOME"),
]

MONEY_RE = re.compile(r"([-]?\$?[\d,]+\.\d{2})")
EIN_RE = re.compile(r"\b(\d{2}-?\d{7})\b")


def extract_text(path: str) -> str:
    if path.lower().endswith((".txt", ".text")):
        return open(path, encoding="utf-8", errors="replace").read()
    if PdfReader is None:
        sys.exit("pypdf not installed — run with: uv run --with pypdf " + sys.argv[0])
    reader = PdfReader(path)
    return "\n".join((page.extract_text() or "") for page in reader.pages)


def split_documents(text: str):
    """Return list of (doc_type, chunk_text)."""
    lines = text.splitlines()
    docs, cur_type, cur = [], None, []
    for line in lines:
        up = line.strip().upper()
        matched = None
        for t, h in DOC_HEADERS:
            # header must be a standalone line (or header + trailing label, no $)
            if re.match(r"^" + re.escape(h) + r"$", up) or (
                up.startswith(h) and "$" not in line
            ):
                matched = (t, h)
                break
        if matched:
            if cur_type:
                docs.append((cur_type, "\n".join(cur)))
            cur_type, cur = matched[0], [line]
        elif cur_type:
            cur.append(line)
    if cur_type:
        docs.append((cur_type, "\n".join(cur)))
    return docs


def _money_on_line(line: str, label: str):
    m = re.search(label + r"[\s.]+([-]?\$?[\d,]+\.\d{2})", line, re.I)
    return float(m.group(1).replace(",", "").replace("$", "")) if m else None


def parse_w2(chunk: str) -> dict:
    labels = {
        "wages": "Wages, tips, other comp",
        "fed_withheld": "Federal income tax withheld",
        "ss_wages": "Social security wages",
        "ss_withheld": "Social security tax withheld",
        "medicare_wages": "Medicare wages and tips",
        "medicare_withheld": "Medicare tax withheld",
        "state_withheld": "State income tax",
    }
    fields = {}
    for line in chunk.splitlines():
        for key, label in labels.items():
            if key not in fields:
                v = _money_on_line(line, label)
                if v is not None:
                    fields[key] = v

    ein = None
    for pat in (r"Employer.?s? identification number[:\s]+([\d\-]+)",
                r"\bEIN[:\s]*([\d\-]{9,10})\b"):
        m = re.search(pat, chunk, re.I)
        if m:
            ein = m.group(1)
            break
    if not ein:
        m = EIN_RE.search(chunk)
        ein = m.group(1) if m else None

    name = None
    m = re.search(r"Employer.?s? name[:\s]+(.+)", chunk, re.I)
    if m:
        name = m.group(1).strip()

    state = None
    m = re.search(r"\bState[:\s]+([A-Z]{2})\b", chunk)
    if m:
        state = m.group(1)

    return {
        "employer": name,
        "ein": ein,
        "wages": fields.get("wages"),
        "fed_withheld": fields.get("fed_withheld"),
        "ss_wages": fields.get("ss_wages"),
        "ss_withheld": fields.get("ss_withheld"),
        "medicare_wages": fields.get("medicare_wages"),
        "medicare_withheld": fields.get("medicare_withheld"),
        "state": state,
        "state_withheld": fields.get("state_withheld"),
    }


def parse_1099(doc_type: str, chunk: str) -> dict:
    payer = None
    m = re.search(r"Payer.?s? name[:\s]+(.+)", chunk, re.I)
    if m:
        payer = m.group(1).strip()
    tin = None
    m = re.search(r"Payer.?s? TIN[:\s]*([\d\-]+)", chunk, re.I)
    if m:
        tin = m.group(1)
    amounts = {}
    for line in chunk.splitlines()[:40]:
        m = re.match(r"([A-Za-z][A-Za-z ,]+?)\s*[.:]+ ?\$?([\d,]+\.\d{2})\s*$", line.strip())
        if m:
            label = m.group(1).strip()
            if label.lower() not in ("document id", "sequence", "form", "page"):
                amounts[label] = float(m.group(2).replace(",", ""))
    return {"type": doc_type, "payer": payer, "tin": tin, "amounts": amounts}


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    text = extract_text(sys.argv[1])
    out = {"w2s": [], "1099s": []}
    for doc_type, chunk in split_documents(text):
        if doc_type == "W-2":
            rec = parse_w2(chunk)
            if rec["wages"] is not None or rec["employer"]:
                out["w2s"].append(rec)
        else:
            rec = parse_1099(doc_type, chunk)
            if rec["payer"] or rec["amounts"]:
                out["1099s"].append(rec)
    year = re.search(r"\b(20\d{2})\b", text)
    out["year_hint"] = year.group(1) if year else None
    print(json.dumps(out, indent=2))
    if len(sys.argv) > 2 and sys.argv[2] == "-o":
        with open(sys.argv[3], "w", encoding="utf-8") as f:
            json.dump(out, f, indent=2)


if __name__ == "__main__":
    main()
