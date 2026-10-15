// Settings, as a drawer over the window: the options that aren't about one
// series (pages, the filename template, the browser window) and the
// catalogs' standing with its Update button.

import { Spinner, XIcon } from './icons';
import { MoreOptions } from './OptionsPanel';
import { catalogSummary } from './search';
import type { Options, State } from './types';

interface Props {
  state: State;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onError: (message: string) => void;
  onUpdateCatalog: () => void;
  onClose: () => void;
}

export function SettingsDrawer({ state, onOptions, onError, onUpdateCatalog, onClose }: Props) {
  const catalog = state.catalog;
  return (
    <section
      className="log-drawer drawer settings-drawer"
      aria-label="Settings"
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          onClose();
        }
      }}
    >
      <div className="log-head">
        <h2>Settings</h2>
        <button type="button" className="btn ghost small icon-only" aria-label="Close the settings" onClick={onClose} autoFocus>
          <XIcon size={14} />
        </button>
      </div>
      <div className="drawer-body">
        <MoreOptions state={state} onOptions={onOptions} onError={onError} />
        <section className="opt-group" aria-labelledby="opt-catalog">
          <h3 id="opt-catalog">Search catalogs</h3>
          <p className="hint">{catalogSummary(catalog, new Date())}</p>
          <p className="hint">
            The catalogs list the titles of every supported site, built nightly. The window refreshes them once a day; press
            Update to fetch what changed now.
          </p>
          <div>
            <button type="button" className="btn small" disabled={catalog.updating} onClick={onUpdateCatalog}>
              {catalog.updating ? (
                <>
                  <Spinner size={11} /> Updating…
                </>
              ) : (
                'Update the catalogs'
              )}
            </button>
          </div>
          {catalog.error !== '' && (
            <p className="error-text small" role="alert">
              {catalog.error}
            </p>
          )}
        </section>
      </div>
    </section>
  );
}
