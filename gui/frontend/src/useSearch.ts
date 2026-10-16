// The Search mode's state: the query and site, the hits the backend answered
// for them (after a short pause while typing), and the site picker's list,
// refreshed when a catalog update lands (state.catalog.generation moves).

import { useCallback, useEffect, useRef, useState } from 'react';
import type { Api } from './api';
import { debounce, SEARCH_DELAY_MS } from './search';
import type { ChapterCount, Hit, SiteInfo } from './types';

/** How many of a search's first results get their chapters counted */
export const COUNT_TOP = 12;
/** How many counts run at once */
const COUNT_AT_ONCE = 3;

export interface Search {
  query: string;
  site: string;
  hits: Hit[];
  searching: boolean;
  sites: SiteInfo[];
  /** The chapter counts fetched for results the catalogs had none for, by URL */
  counts: Record<string, ChapterCount>;
  setQuery: (query: string) => void;
  setSite: (site: string) => void;
}

export function useSearch(api: Api, generation: number, active: boolean): Search {
  const [query, setQueryState] = useState('');
  const [site, setSiteState] = useState('');
  const [hits, setHits] = useState<Hit[]>([]);
  const [searching, setSearching] = useState(false);
  const [sites, setSites] = useState<SiteInfo[]>([]);
  const [counts, setCounts] = useState<Record<string, ChapterCount>>({});
  // the newest request wins: an older answer arriving later is dropped
  const seq = useRef(0);
  // the URLs counted or being counted, so a series is fetched once
  const asked = useRef(new Set<string>());

  // the first results from a site the app can read, with no count yet, are
  // counted a few at a time; a newer search stops the ones not started
  const countTop = useCallback(
    (found: Hit[], n: number) => {
      const todo = found
        .slice(0, COUNT_TOP)
        .filter((h) => h.downloadable && h.chapters === 0 && !asked.current.has(h.url));
      if (todo.length === 0) {
        return;
      }
      todo.forEach((h) => asked.current.add(h.url));
      setCounts((c) => ({ ...c, ...Object.fromEntries(todo.map((h) => [h.url, 'counting' as const])) }));
      let next = 0;
      const worker = async () => {
        while (next < todo.length) {
          const h = todo[next++];
          if (n !== seq.current) {
            // left for a later search to ask again
            asked.current.delete(h.url);
            setCounts((c) => {
              const { [h.url]: _, ...rest } = c;
              return rest;
            });
            continue;
          }
          let result: ChapterCount;
          try {
            result = await api.countChapters(h.url);
          } catch {
            result = 'failed';
          }
          setCounts((c) => ({ ...c, [h.url]: result }));
        }
      };
      for (let i = 0; i < COUNT_AT_ONCE; i++) {
        void worker();
      }
    },
    [api],
  );

  const run = useCallback(
    (q: string, s: string) => {
      const n = ++seq.current;
      if (q.trim() === '') {
        setHits([]);
        setSearching(false);
        return;
      }
      setSearching(true);
      api
        .search(q, s)
        .then((found) => {
          if (n === seq.current) {
            setHits(found);
            setSearching(false);
            countTop(found, n);
          }
        })
        .catch(() => {
          if (n === seq.current) {
            setHits([]);
            setSearching(false);
          }
        });
    },
    [api, countTop],
  );
  // the pause calls whatever run is current, so a hot reload (or a new api)
  // never leaves it holding an old closure
  const latest = useRef(run);
  latest.current = run;
  const [delayed] = useState(() => debounce((q: string, s: string) => latest.current(q, s), SEARCH_DELAY_MS));
  useEffect(() => () => delayed.cancel(), [delayed]);

  const setQuery = (q: string) => {
    setQueryState(q);
    delayed.call(q, site);
  };
  const setSite = (s: string) => {
    setSiteState(s);
    delayed.cancel();
    run(query, s);
  };

  // the picker's sites, and the hits, follow the catalog on disk
  useEffect(() => {
    if (!active) {
      return;
    }
    let gone = false;
    api
      .sites()
      .then((list) => {
        if (!gone) {
          setSites(list);
        }
      })
      .catch(() => undefined);
    if (query.trim() !== '') {
      run(query, site);
    }
    return () => {
      gone = true;
    };
    // query and site are read, not followed: a new catalog re-runs the current ones
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, generation, active]);

  return { query, site, hits, searching, sites, counts, setQuery, setSite };
}
