# Site check

A liveness probe of every site in the README's "Supported sites" list: does
the home page answer, is it parked, is it behind a Cloudflare challenge,
and which platform family does its markup look like. It is how the site
table in `catalog/sites.json` was first filled, and what to rerun before
touching that table.

```bash
python3 -I tools/site-check/probe.py README.md      # writes results.json
python3 -I tools/site-check/fingerprint.py          # adds platform + catalog link
```

A site answering is not proof its grabber works: that is what the makefile
smoke targets are for. The last report is `report-2026-10-15.md`.
