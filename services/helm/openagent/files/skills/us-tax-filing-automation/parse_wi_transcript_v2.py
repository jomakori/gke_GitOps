#!/usr/bin/env python3
"""Parse IRS Wage & Income transcripts (OLD + NEW 2023+ formats) into JSON.

The 2023+ transcript layout differs from the legacy one:
- section headers are "Form W-2 Wage and Tax Statement", "Form 1099-INT", ...
- EINs are masked to last 4 (XX-XXX8068), names truncated to ~14 chars
- amounts carry $ and commas
Handles both formats; masked/truncated values are kept as-is with a flag.

Usage:
    uv run --with pypdf parse_wi_transcript_v2.py transcript.pdf [-o out.json]
"""
import json
import re
import sys

try:
    from pypdf import PdfReader
except ImportError:
    PdfReader = None

DOC_HEADERS = [
    ("W-2", r"Form\s+W-2\s+Wage and Tax Statement"),
    ("SSA", r"Form\s+SSA-?1099|Social Security Benefit Statement"),
    ("1099-INT", r"Form\s+1099-INT"),
    ("1099-DIV", r"Form\s+1099-DIV"),
    ("1099-NEC", r"Form\s+1099-NEC"),
    ("1099-MISC", r"Form\s+1099-MISC"),
    ("1099-G", r"Form\s+1099-G\b"),
    ("1099-R", r"Form\s+1099-R"),
    ("1099-B", r"Form\s+1099-B"),
    ("1099-SA", r"Form\s+5498-SA or 5498-MSA"),
    ("5498", r"Form\s+5498\b"),
    ("1099-K", r"Form\s+1099-K"),
    ("1099-C", r"Form\s+1099-C"),
]
LEGACY_HEADERS = [
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
MASKED_EIN_RE = re.compile(r"\bXX-XXX(\d{4})\b")


def extract_text(path: str) -> str:
    if path.lower().endswith((".txt", ".text")):
        return open(path, encoding="utf-8", errors="replace").read()
    if PdfReader is None:
        sys.exit("pypdf not installed — run with: uv run --with pypdf " + sys.argv[0])
    reader = PdfReader(path)
    return "\n".join((page.extract_text() or "") for page in reader.pages)


def is_new_format(text: str) -> bool:
    return bool(re.search(r"Form\s+W-2\s+Wage and Tax Statement", text)) or \
           bool(re.search(r"Tax Period Requested", text))


def split_documents(text: str):
    lines = text.splitlines()
    docs, cur_type, cur = [], None, []
    new_fmt = is_new_format(text)
    for line in lines:
        matched = None
        if new_fmt:
            for t, pat in DOC_HEADERS:
                if re.search(pat, line, re.I):
                    matched = (t, pat)
                    break
        else:
            up = line.strip().upper()
            for t, h in LEGACY_HEADERS:
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
    m = re.search(r"(?:" + label + r")[\s.:]+([-]?\$?[\d,]+\.\d{2})", line, re.I)
    return float(m.group(1).replace(",", "").replace("$", "")) if m else None


def parse_w2(chunk: str) -> dict:
    labels = {
        "wages": r"Wages, Tips and Other Compensation|Wages, tips, other comp",
        "fed_withheld": r"Federal Income Tax Withheld",
        "ss_wages": r"Social Security Wages",
        "ss_withheld": r"Social Security Tax Withheld",
        "medicare_wages": r"Medicare Wages and Tips",
        "medicare_withheld": r"Medicare Tax Withheld",
        "deferred_comp": r"Deferred Compensation",
        "dd_health": r"Code \"DD\" Cost of Employer-Sponsored Health Coverage",
        "state_withheld": r"State Income Tax",
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

    masked_ein = None
    m = MASKED_EIN_RE.search(chunk)
    if m:
        masked_ein = m.group(1)

    name = None
    m = re.search(r"Employer name[: ]+(.+)", chunk, re.I)
    if m:
        name = m.group(1).strip()
    if not name:
        # new format: name line comes right after the EIN line
        lines = chunk.splitlines()
        for i, ln in enumerate(lines):
            if "Employer Identification Number" in ln and i + 1 < len(lines):
                candidate = lines[i + 1].strip()
                if candidate and not candidate.startswith("Employee"):
                    name = candidate
                    break

    state = None
    m = re.search(r"\bState[: ]+([A-Z]{2})\b", chunk)
    if m:
        state = m.group(1)

    return {
        "employer": name,
        "ein": ein,
        "ein_masked_last4": masked_ein,
        "wages": fields.get("wages"),
        "fed_withheld": fields.get("fed_withheld"),
        "ss_wages": fields.get("ss_wages"),
        "ss_withheld": fields.get("ss_withheld"),
        "medicare_wages": fields.get("medicare_wages"),
        "medicare_withheld": fields.get("medicare_withheld"),
        "deferred_comp": fields.get("deferred_comp"),
        "dd_health": fields.get("dd_health"),
        "state": state,
        "state_withheld": fields.get("state_withheld"),
    }


def parse_1099(doc_type: str, chunk: str) -> dict:
    payer = None
    m = re.search(r"Payer name[: ]+(.+)", chunk, re.I)
    if m:
        payer = m.group(1).strip()
    if not payer:
        m = re.search(r"Trustee name[: ]+(.+)", chunk, re.I)
        if m:
            payer = m.group(1).strip()
    if not payer:
        lines = chunk.splitlines()
        for i, ln in enumerate(lines):
            if ("Federal Identification Number" in ln or "TIN" in ln) and i + 1 < len(lines):
                candidate = lines[i + 1].strip()
                if candidate and "Recipient" not in candidate and "Participant" not in candidate:
                    payer = candidate
                    break

    tin = None
    m = re.search(r"(?:Payer|Trustee).?s? (?:TIN|Federal Identification Number|FIN)[:\s]*([\d\-]+)", chunk, re.I)
    if m:
        tin = m.group(1)
    masked_tin = None
    m = MASKED_EIN_RE.search(chunk)
    if m:
        masked_tin = m.group(1)

    amounts = {}
    for line in chunk.splitlines():
        m = re.match(r"([A-Za-z][A-Za-z ,()'\"-]+?)[\s.:]+\$?([\d,]+\.\d{2})\s*$", line.strip())
        if m:
            label = m.group(1).strip()
            if label.lower() not in ("document id", "sequence", "form", "page"):
                amounts[label] = float(m.group(2).replace(",", ""))
    return {"type": doc_type, "payer": payer, "tin": tin, "tin_masked_last4": masked_tin, "amounts": amounts}


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    text = extract_text(sys.argv[1])
    out = {"w2s": [], "1099s": [], "format": "new" if is_new_format(text) else "legacy"}
    for doc_type, chunk in split_documents(text):
        if doc_type == "W-2":
            rec = parse_w2(chunk)
            if rec["wages"] is not None or rec["employer"] or rec["ein_masked_last4"]:
                out["w2s"].append(rec)
        else:
            rec = parse_1099(doc_type, chunk)
            if rec["payer"] or rec["amounts"] or rec["tin_masked_last4"]:
                out["1099s"].append(rec)
    m = re.search(r"Tax Period Requested: (?:12-31-)?(20\d{2})", text) or \
        re.search(r"\b(20\d{2})\b", text)
    out["year_hint"] = m.group(1) if m else None
    print(json.dumps(out, indent=2))
    if len(sys.argv) > 2 and sys.argv[2] == "-o":
        with open(sys.argv[3], "w", encoding="utf-8") as f:
            json.dump(out, f, indent=2)


if __name__ == "__main__":
    main()
