// The window. Top to bottom:
//
//   top bar     always there: the brand (home), one box that searches a
//               title or loads a link, Check for new chapters, Settings
//   page        the library (home), the search results (while the box holds
//               a title), or the series: its chapters, with what belongs to
//               it on the right (facts, Follow, Save as, Download)
//   strip       what's happening now: the running download and the ones
//               waiting their turn, then how the last one ended
//   status bar  the latest log line (the full log opens as a drawer), version
//
// Settings and the download's details open as drawers over the window. A
// download never locks the window: the series, the search and the library
// stay usable, and Download on another series queues it (see queue.go).

import { useEffect, useRef, useState } from 'react';
import { ActivityDrawer } from './ActivityDrawer';
import type { Api } from './api';
import { BrowserAlert } from './BrowserAlert';
import { ChapterPanel } from './ChapterPanel';
import { errorMessage } from './format';
import { Spinner } from './icons';
import { followedOf, isLinkQuery, urlKey, type Sort } from './library';
import { LibraryPanel } from './LibraryPanel';
import { LogDrawer, StatusBar } from './LogPanel';
import { Mascot } from './Mascot';
import { createOptionsWriter } from './optionsWriter';
import { ProgressStrip } from './ProgressStrip';
import { type BlockedHit, ResultsPanel } from './ResultsPanel';
import { runningHere } from './run';
import { SeriesAside } from './SeriesAside';
import { SettingsDrawer } from './SettingsDrawer';
import { TopBar } from './TopBar';
import type { Followed, Hit, Options, State } from './types';
import { useSearch } from './useSearch';
import { useWindow } from './useWindow';

/** The page under the top bar: the library, or the series */
type Page = 'home' | 'series';

export function App({ api }: { api: Api }) {
  const { state, chapters, selected, log, failure } = useWindow(api);
  const [page, setPage] = useState<Page>('home');
  const [sort, setSort] = useState<Sort>('new');
  // the top box: a title searches the catalogs (the results page shows
  // while it holds one); a link loads
  const search = useSearch(api, state?.catalog.generation ?? 0, true);
  const results = search.query.trim() !== '' && !isLinkQuery(search.query);
  // a picked hit from a site the downloader has no grabber for, explained in
  // the results instead of loaded
  const [blocked, setBlocked] = useState<BlockedHit | null>(null);
  // "Copied" on the note's button for a moment after a copy
  const [copied, setCopied] = useState(false);
  const copiedTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(copiedTimer.current), []);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [activityOpen, setActivityOpen] = useState(false);
  const [logOpen, setLogOpen] = useState(false);
  // the finished run's strip, until dismissed; a new run shows it again
  const [stripDismissed, setStripDismissed] = useState(false);
  const [loadRefusal, setLoadRefusal] = useState('');
  // the series asked for, until the backend's state names it: the page
  // shows it loading from the click, not the previous series
  const [pending, setPending] = useState('');
  // a series asked for while another one loads: loaded next, once that
  // load ends, instead of refused
  const [queued, setQueued] = useState('');
  const [actionError, setActionError] = useState('');
  const [optionError, setOptionError] = useState('');
  // "Added to the queue" under the Download button for a moment
  const [queuedNote, setQueuedNote] = useState('');
  const noteTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(noteTimer.current), []);
  const noteQueued = (queued: boolean) => {
    if (!queued) {
      return;
    }
    setQueuedNote('Added to the queue: it starts when the download running is done.');
    clearTimeout(noteTimer.current);
    noteTimer.current = setTimeout(() => setQueuedNote(''), 4000);
  };
  // options are written one at a time, each patch over the last set sent
  const [optionsWriter] = useState(() =>
    createOptionsWriter((options, load) =>
      load === undefined ? api.setOptions(options) : api.setOptionsAndLoad(options, load),
    ),
  );

  useEffect(() => {
    if (state !== null) {
      optionsWriter.pushed(state.options);
    }
  }, [state, optionsWriter]);

  // the backend named the series asked for: its state speaks from here on
  const stateURL = state?.url ?? '';
  useEffect(() => {
    if (pending !== '' && urlKey(stateURL) === urlKey(pending)) {
      setPending('');
    }
  }, [stateURL, pending]);

  // the load that waited for the one before it
  const loading = state?.loading ?? false;
  useEffect(() => {
    if (queued === '' || loading) {
      return;
    }
    const target = queued;
    setQueued('');
    api.load(target).catch((e: unknown) => {
      setPending('');
      setLoadRefusal(errorMessage(e));
    });
  }, [queued, loading, api]);

  // a refusal is about the moment it was made: a load that starts clears it
  useEffect(() => {
    if (loading) {
      setLoadRefusal('');
    }
  }, [loading]);

  // a run that starts brings its strip back
  const running = state?.running ?? false;
  useEffect(() => {
    if (running) {
      setStripDismissed(false);
    }
  }, [running]);

  if (state === null) {
    return (
      <main className="boot">
        <Mascot expression="soft" size={48} />
        {failure !== '' ? (
          <p className="error-text" role="alert">
            {failure}
          </p>
        ) : (
          <p className="muted">
            <Spinner /> Starting…
          </p>
        )}
      </main>
    );
  }

  // only a load locks the series page; a download works on its own copies
  const busy = state.loading;
  const library = state.library;
  const libraryBusy = busy || library.checking;
  // until the backend's state names the series asked for, the series page
  // shows that one loading, never the previous series under its name
  const target = pending !== '' && urlKey(pending) !== urlKey(state.url) ? pending : '';
  const view: State =
    target === ''
      ? state
      : { ...state, url: target, loading: true, loaded: false, loadError: '', title: '', cover: '', chapterCount: 0, selectedCount: 0 };
  const followed = followedOf(library, view.url);
  const fail = (set: (m: string) => void) => (e: unknown) => set(errorMessage(e));

  // opens the series page of target and loads it. While another series
  // loads, target waits its turn (the page shows it loading meanwhile);
  // while the library is checked nothing can load, and the message says what
  // to do. A download running doesn't stop a load: it has its own copies.
  const load = (target: string) => {
    setLoadRefusal('');
    setActionError('');
    if (library.checking) {
      setLoadRefusal('The library is being checked for new chapters. Wait for it to finish, or stop it, before opening a series.');
      return;
    }
    setPage('series');
    search.setQuery('');
    if (state.loading) {
      // the series loading already: nothing to queue, its page is up
      if (urlKey(target) === urlKey(state.url)) {
        setPending('');
        setQueued('');
        return;
      }
      setPending(target);
      setQueued(target);
      return;
    }
    setPending(target);
    api.load(target).catch((e: unknown) => {
      setPending('');
      fail(setLoadRefusal)(e);
    });
  };
  // the series page of url: loaded unless it's the loaded one already
  const openSeries = (url: string) => {
    if (urlKey(url) === urlKey(state.url) && (state.loaded || state.loading)) {
      setPage('series');
      search.setQuery('');
      return;
    }
    load(url);
  };
  const setOptions = (patch: Partial<Options>) => {
    setOptionError('');
    return optionsWriter.patch(patch).catch((e: unknown) => {
      setOptionError(errorMessage(e));
      throw e;
    });
  };
  // a filter picked from a selector applies at once: it's written with the
  // reload of the loaded series, so it's never shown as not applied
  const pickFilter = (patch: Partial<Options>) => {
    setOptionError('');
    return optionsWriter.patch(patch, state.url).catch((e: unknown) => {
      setOptionError(errorMessage(e));
    });
  };
  const follow = (f: Followed) => {
    setActionError('');
    api.follow(f).catch(fail(setActionError));
  };
  const followHit = (hit: Hit) => {
    follow({
      url: hit.url,
      title: hit.title,
      site: hit.site,
      downloadable: hit.downloadable,
      chapters: hit.chapters,
      latest: 0,
      savedUpTo: 0,
      saved: [],
      everSaved: [],
      known: [],
      baselined: false,
      new: 0,
      followedAt: '',
      checkedAt: '',
      lastSaved: '',
      error: '',
      cover: '',
    });
  };
  const followLoaded = () => {
    follow({
      url: state.url,
      title: state.title,
      site: '',
      downloadable: true,
      chapters: state.chapterCount,
      latest: 0,
      savedUpTo: 0,
      saved: [],
      everSaved: [],
      known: [],
      baselined: false,
      new: 0,
      followedAt: '',
      checkedAt: '',
      lastSaved: '',
      error: '',
      cover: '',
    });
  };
  const unfollow = (url: string) => {
    setActionError('');
    api.unfollow(url).catch(fail(setActionError));
  };
  const problem = loadRefusal !== '' ? loadRefusal : optionError;

  return (
    <div className="app">
      <TopBar
        state={state}
        query={search.query}
        onQuery={(text) => {
          search.setQuery(text);
          if (text.trim() === '') {
            setBlocked(null);
          }
        }}
        onLoad={load}
        onHome={() => {
          setPage('home');
          search.setQuery('');
        }}
        onCheck={() => {
          setActionError('');
          api.checkLibrary().catch(fail(setActionError));
        }}
        onCancelCheck={() => {
          api.cancelCheck().catch(() => undefined);
        }}
        onSettings={() => setSettingsOpen((o) => !o)}
        settingsOpen={settingsOpen}
        problem={problem}
        onDismiss={() => {
          setLoadRefusal('');
          setOptionError('');
        }}
      />

      {results ? (
        <main className="page results-page">
          <ResultsPanel
            catalog={state.catalog}
            sites={search.sites}
            query={search.query}
            site={search.site}
            hits={search.hits}
            counts={search.counts}
            searching={search.searching}
            busy={busy}
            blocked={blocked}
            copied={copied}
            onSite={search.setSite}
            onPick={(hit) => {
              if (!hit.downloadable) {
                setBlocked({ hit, query: search.query, site: search.site });
                setCopied(false);
                return;
              }
              setBlocked(null);
              load(hit.url);
            }}
            onFollow={followHit}
            onUnfollow={unfollow}
            onUpdate={() => {
              api.updateCatalog().catch(fail(setActionError));
            }}
            onCopy={(url) => {
              api
                .copyText(url)
                .then(() => {
                  setCopied(true);
                  clearTimeout(copiedTimer.current);
                  copiedTimer.current = setTimeout(() => setCopied(false), 1500);
                })
                .catch(fail(setActionError));
            }}
            onOpen={(url) => api.openURL(url)}
            onDismissBlocked={() => {
              setBlocked(null);
              setCopied(false);
            }}
          />
        </main>
      ) : page === 'home' ? (
        <main className="page home">
          <LibraryPanel
            state={state}
            busy={libraryBusy}
            sort={sort}
            onSort={setSort}
            onOpen={(f) => openSeries(f.url)}
            onDownloadNew={(url) => {
              setActionError('');
              api.downloadNew(url).then(noteQueued).catch(fail(setActionError));
            }}
            onDownloadAllNew={() => {
              setActionError('');
              api.downloadAllNew().catch(fail(setActionError));
            }}
            onUnfollow={(f) => unfollow(f.url)}
            onOpenURL={(url) => api.openURL(url)}
            onReveal={(url) => {
              setActionError('');
              api.revealSaved(url).catch(fail(setActionError));
            }}
          />
          {actionError !== '' && (
            <p className="error-text small page-error" role="alert">
              {actionError}
            </p>
          )}
        </main>
      ) : (
        <main className="workspace series-page">
          <nav className="crumbs" aria-label="Breadcrumb">
            <button
              type="button"
              className="btn ghost small"
              onClick={() => {
                setPage('home');
              }}
            >
              ← Library
            </button>
            {view.title !== '' && (
              <>
                <span className="muted" aria-hidden="true">
                  /
                </span>
                <span className="muted crumb-title">{view.title}</span>
              </>
            )}
          </nav>
          <ChapterPanel
            state={view}
            chapters={target === '' ? chapters : []}
            selected={target === '' ? selected : []}
            disabled={busy}
            followed={followed}
            onToggle={(index, on) => api.setSelected(index, on)}
            onToggleMany={(indices, on) => api.setSelectedMany(indices, on)}
            onSetAll={(on) => api.setAll(on)}
            onRange={(text) => api.selectRange(text)}
            onVolumes={(text) => api.setVolumes(text)}
            onReload={() => load(view.url)}
            onError={setActionError}
            onPick={pickFilter}
            onOptions={setOptions}
          />
          <SeriesAside
            state={view}
            chapters={target === '' ? chapters : []}
            selected={target === '' ? selected : []}
            followed={followed}
            actionError={actionError}
            note={queuedNote}
            onFollow={followLoaded}
            onUnfollow={() => unfollow(view.url)}
            onOpenURL={(url) => api.openURL(url)}
            onOptions={setOptions}
            onChooseFolder={async () => {
              setOptionError('');
              await api.chooseFolder();
            }}
            onError={setOptionError}
            onStart={() => {
              setActionError('');
              api.start().then(noteQueued).catch(fail(setActionError));
            }}
            onStartNew={(indices) => {
              setActionError('');
              api
                .setSelectedMany(indices, true)
                .then(() => api.start())
                .then(noteQueued)
                .catch(fail(setActionError));
            }}
            onDetails={() => setActivityOpen(true)}
            onCancel={() => {
              api.cancel().catch(fail(setActionError));
            }}
          />
        </main>
      )}

      <ProgressStrip
        state={state}
        dismissed={stripDismissed}
        // the series page's dock shows the run of its own series: one progress bar, not two
        runShownElsewhere={!results && page !== 'home' && target === '' && runningHere(state)}
        onDetails={() => setActivityOpen(true)}
        onCancel={() => {
          api.cancel().catch(fail(setActionError));
        }}
        onRemoveQueued={(id) => {
          api.removeQueued(id).catch(fail(setActionError));
        }}
        onOpenFolder={() => {
          api.openFolder().catch(fail(setActionError));
        }}
        onDismiss={() => setStripDismissed(true)}
      />

      {settingsOpen && (
        <SettingsDrawer
          state={state}
          onOptions={setOptions}
          onError={setOptionError}
          onUpdateCatalog={() => {
            api.updateCatalog().catch(fail(setActionError));
          }}
          onClose={() => setSettingsOpen(false)}
        />
      )}
      {activityOpen && (
        <ActivityDrawer
          state={state}
          onOpenFolder={() => {
            api.openFolder().catch(fail(setActionError));
          }}
          onRemoveQueued={(id) => {
            api.removeQueued(id).catch(fail(setActionError));
          }}
          onClose={() => setActivityOpen(false)}
        />
      )}
      {logOpen && <LogDrawer log={log} onClose={() => setLogOpen(false)} />}
      {state.browserAlert !== '' && (
        <BrowserAlert
          message={state.browserAlert}
          onShow={() => {
            api.showBrowser().catch(() => undefined);
          }}
          onDismiss={() => {
            api.dismissBrowserAlert().catch(() => undefined);
          }}
        />
      )}
      <StatusBar
        log={log}
        version={state.version}
        releaseURL={state.releaseURL}
        update={state.update}
        open={logOpen}
        onToggle={() => setLogOpen((o) => !o)}
        onOpenURL={(url) => api.openURL(url)}
        onUpdate={() => api.update()}
      />
    </div>
  );
}
