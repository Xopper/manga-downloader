import json, re, ssl, urllib.request, urllib.error
from concurrent.futures import ThreadPoolExecutor
UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15"
ctx = ssl.create_default_context()
rows = json.load(open("results.json"))
MARK = [
 ("madara", r"wp-manga|madara"),
 ("themesia", r"ts-breadcrumb|mangastream|class=\"bixbox|/wp-content/themes/mangareader|chapterlist"),
 ("wordpress", r"/wp-content/"),
 ("astro", r"data-astro"),
 ("nextjs", r"__NEXT_DATA__|/_next/"),
 ("nuxt", r"__NUXT__|/_nuxt/"),
 ("foolslide", r"foolslide|FoOlSlide"),
 ("blogger", r"blogger\.com|blogspot"),
]
def fp(r):
    if r["verdict"] != "ok": r["platform"] = "-"; return r
    try:
        req = urllib.request.Request(r["final"], headers={"User-Agent": UA})
        with urllib.request.urlopen(req, timeout=25, context=ctx) as h: t = h.read(600000).decode("utf-8","ignore")
    except Exception: r["platform"] = "?"; return r
    r["platform"] = next((n for n,p in MARK if re.search(p, t, re.I)), "custom")
    # a catalog/all-manga link on the home page
    m = re.search(r'href="([^"]*/(manga|manga-list|series|comics|comic|list|directory|library|titles|webtoons?)/?(\?[^"]*)?)"', t, re.I)
    r["catalog"] = m.group(1)[:60] if m else ""
    return r
with ThreadPoolExecutor(16) as ex: rows = list(ex.map(fp, rows))
for r in rows:
    if r["site"] == "komikindo.ch": r["verdict"] = "ok"; r["note"] = ""
json.dump(rows, open("results.json","w"), indent=1, ensure_ascii=False)
from collections import Counter
print(Counter(r.get("platform") for r in rows))
print("no catalog link:", [r["site"] for r in rows if r["verdict"]=="ok" and not r.get("catalog")])
