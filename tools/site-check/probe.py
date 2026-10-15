import re, json, sys, ssl, socket, urllib.request, urllib.error, html
from concurrent.futures import ThreadPoolExecutor
from urllib.parse import urlparse

readme = open(sys.argv[1], encoding="utf-8").read()
sec = readme.split("## Supported sites",1)[1].split("</details>",1)[0]
sites = re.findall(r"^- \[(.+?)\]\((https?://[^\)]+)\)( \\\*)?", sec, re.M)

UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15"
ctx = ssl.create_default_context()

def fetch(url):
    req = urllib.request.Request(url, headers={"User-Agent": UA, "Accept": "text/html,*/*;q=0.8", "Accept-Language": "en-US,en;q=0.9"})
    try:
        with urllib.request.urlopen(req, timeout=25, context=ctx) as r:
            body = r.read(400000)
            return r.status, r.geturl(), dict(r.headers), body
    except urllib.error.HTTPError as e:
        try: body = e.read(400000)
        except Exception: body = b""
        return e.code, e.geturl() or url, dict(e.headers or {}), body
    except Exception as e:
        return None, url, {}, repr(e).encode()

def classify(label, url, needs_browser):
    status, final, hdr, body = fetch(url)
    text = body.decode("utf-8","ignore")
    m = re.search(r"<title[^>]*>(.*?)</title>", text, re.S|re.I)
    title = html.unescape(re.sub(r"\s+"," ", m.group(1))).strip()[:90] if m else ""
    host = urlparse(url).netloc.replace("www.","")
    fhost = urlparse(final).netloc.replace("www.","") if final else ""
    note = ""
    if status is None:
        verdict = "dead"; note = text[:120]
    elif "just a moment" in title.lower() or hdr.get("cf-mitigated") == "challenge":
        verdict = "cloudflare"
    elif status in (401,403,503) and "cloudflare" in text.lower():
        verdict = "cloudflare"
    elif status >= 500:
        verdict = "error"; note = f"HTTP {status}"
    elif status == 404 or status in (401,403):
        verdict = "error"; note = f"HTTP {status}"
    elif re.search(r"domain (is )?for sale|parked|buy this domain|sedo|godaddy|namecheap|expired", title+text[:3000], re.I) or len(body) < 1500:
        verdict = "parked"; note = f"{len(body)} bytes"
    else:
        verdict = "ok"
    if fhost and fhost != host:
        note = (note + " " if note else "") + f"redirects to {fhost}"
    return {"site": host, "label": label, "url": url, "browser": bool(needs_browser), "status": status, "final": final, "title": title, "verdict": verdict, "note": note}

with ThreadPoolExecutor(16) as ex:
    rows = list(ex.map(lambda s: classify(*s), sites))
json.dump(rows, open("results.json","w"), indent=1, ensure_ascii=False)
from collections import Counter
print(Counter(r["verdict"] for r in rows))
for r in rows:
    if r["verdict"] != "ok" or r["note"]:
        print(f'{r["verdict"]:10} {r["site"]:32} {str(r["status"]):4} {r["title"][:50]!r} {r["note"]}')
