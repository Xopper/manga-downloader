// The Search mode's state: the query and site, the hits the backend answered
// for them (after a short pause while typing), and the site picker's list,
// refreshed when a catalog update lands (state.catalog.generation moves).

import { useCallback, useEffect, useRef, useState } from 'react';
import type { Api } from './api';
import { debounce, SEARCH_DELAY_MS } from './search';
import type { Hit, SiteInfo } from './types';

export interface Search {
  query: string;
  site: string;
  hits: Hit[];
  searching: boolean;
  sites: SiteInfo[];
  setQuery: (query: string) => void;
  setSite: (site: string) => void;
}

export function useSearch(api: Api, generation: number, active: boolean): Search {
  const [query, setQueryState] = useState('');
  const [site, setSiteState] = useState('');
  const [hits, setHits] = useState<Hit[]>([]);
  const [searching, setSearching] = useState(false);
  const [sites, setSites] = useState<SiteInfo[]>([]);
  // the newest request wins: an older answer arriving later is dropped
  const seq = useRef(0);

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
          }
        })
        .catch(() => {
          if (n === seq.current) {
            setHits([]);
            setSearching(false);
          }
        });
    },
    [api],
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

  return { query, site, hits, searching, sites, setQuery, setSite };
}
