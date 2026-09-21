#!/usr/bin/env python3
"""Fetch WeChat doc pages and convert main content to markdown-ish text."""
import sys, os, re, html, subprocess, hashlib

CACHE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "cache")
os.makedirs(CACHE, exist_ok=True)


def get(url):
    h = hashlib.md5(url.encode()).hexdigest()
    p = os.path.join(CACHE, h + ".html")
    if not os.path.exists(p) or os.path.getsize(p) < 100:
        r = subprocess.run(["curl", "-sL", "-m", "40", "-A",
                            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120 Safari/537.36",
                            url, "-o", p], capture_output=True)
    return open(p, encoding="utf-8", errors="replace").read()


def strip_tags(s):
    s = re.sub(r"<br\s*/?>", "\n", s)
    s = re.sub(r"</(p|div|li|tr|h1|h2|h3|h4|h5|pre)>", "\n", s)
    s = re.sub(r"<[^>]+>", "", s)
    return html.unescape(s)


def table_md(tbl):
    rows = re.findall(r"<tr[^>]*>(.*?)</tr>", tbl, re.S)
    out = []
    for i, r in enumerate(rows):
        cells = re.findall(r"<t[hd][^>]*>(.*?)</t[hd]>", r, re.S)
        vals = [" ".join(strip_tags(c).split()) for c in cells]
        out.append("| " + " | ".join(vals) + " |")
        if i == 0:
            out.append("|" + "---|" * len(vals))
    return "\n".join(out)


def extract(url):
    s = get(url)
    m = re.search(r'<div id="docContent"[^>]*>(.*?)</div>\s*(?:<div class="related|</main>|</div>\s*</div>\s*</div>)', s, re.S)
    if not m:
        m = re.search(r'<div id="docContent"[^>]*>(.*)', s, re.S)
    body = m.group(1) if m else s
    # cut trailing nav
    body = re.split(r'关于腾讯|Copyright ©', body)[0]
    # protect tables and pre
    placeholders = {}

    def repl(m2):
        k = "\x00%d\x00" % len(placeholders)
        placeholders[k] = m2.group(0)
        return k

    body = re.sub(r"<table.*?</table>", repl, body, flags=re.S)
    body = re.sub(r"<pre.*?</pre>", repl, body, flags=re.S)
    body = re.sub(r"<h([1-6])[^>]*>(.*?)</h\1>", lambda m2: "\n" + "#" * int(m2.group(1)) + " " + strip_tags(m2.group(2)).strip() + "\n", body, flags=re.S)
    txt = strip_tags(body)
    lines = [l.rstrip() for l in txt.split("\n")]
    res = []
    for l in lines:
        if l.strip() in placeholders:
            res.append(l.strip())
        else:
            res.append(l)
    txt = "\n".join(res)
    txt = re.sub(r"\n{3,}", "\n\n", txt)
    # unescape placeholders inside (pre kept raw html -> convert)
    for k, v in placeholders.items():
        if v.lower().startswith("<pre"):
            v2 = strip_tags(re.sub(r"<code[^>]*>|</code>", "", v))
            txt = txt.replace(k, "\n```\n" + v2.strip() + "\n```\n")
        else:
            txt = txt.replace(k, "\n" + table_md(v) + "\n")
    return txt.strip()


if __name__ == "__main__":
    for u in sys.argv[1:]:
        print("=" * 100)
        print("URL:", u)
        print("=" * 100)
        try:
            print(extract(u))
        except Exception as e:
            print("ERROR", e)
