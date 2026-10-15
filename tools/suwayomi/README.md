# Suwayomi for the catalogs

`manga-downloader catalog crawl-suwayomi` builds catalogs through a
[Suwayomi-Server](https://github.com/Suwayomi/Suwayomi-Server): the server
runs the Tachiyomi extensions (from the keiyoushi store), and the command
walks every source's listings through its REST API. Nothing of this is
needed to use the app; it is how the published catalogs are built.

```bash
tools/suwayomi/run.sh                      # downloads the JAR once, starts the server, adds the store
manga-downloader catalog crawl-suwayomi --lang en --out ./catalog --max-pages 2 --source MangaPill
manga-downloader catalog index --out ./catalog
tools/suwayomi/stop.sh
```

Java 21 or newer is needed. The data (extensions, database, log) lives in
`~/.cache/manga-downloader-suwayomi`; `SUWAYOMI_HOME`, `SUWAYOMI_PORT`,
`SUWAYOMI_VERSION` and `SUWAYOMI_PARALLEL` change the defaults. The nightly
job (`.github/workflows/catalog-suwayomi.yaml`) does the same on GitHub,
sharded four ways.
