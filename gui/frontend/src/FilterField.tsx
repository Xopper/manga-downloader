// The language and scanlator filters of the loaded series: free text until
// the first load, then the site's own selectors. A picked value reloads at
// once (onPick); a typed one applies at the next load, and the row says so.

import { SelectField, TextField } from './fields';
import { filterControl, filtersIdentified, filterOptions, shownSelection, type FilterControl, type FilterKind } from './filters';
import { AlertIcon, RefreshIcon } from './icons';
import type { FilterChoice, Options, State } from './types';

interface RowProps {
  state: State;
  busy: boolean;
  /** A filter picked from a selector: applied at once, with the series' reload */
  onPick: (patch: Partial<Options>) => Promise<void>;
  /** A typed filter: saved, applied at the next load */
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onError: (message: string) => void;
  onReload: () => void;
}

/**
 * Both filters side by side, with the "not applied" note when the list is
 * stale. A filter the loaded site has no choice for is left out, and the
 * row with it when nothing is left to show.
 */
export function FilterRow({ state, busy, onPick, onOptions, onError, onReload }: RowProps) {
  const identified = filtersIdentified(state);
  const languageStale = state.loaded && state.options.language !== state.loadedLanguage;
  const scanlatorStale = state.loaded && state.options.scanlator !== state.loadedScanlator;
  const pick = (patch: Partial<Options>) => {
    void onPick(patch);
  };
  const shown = (choice: FilterChoice | null) => filterControl(state, choice) !== 'disabled';
  if (!shown(state.languages) && !shown(state.scanlators) && !state.stale && identified) {
    return null;
  }

  return (
    <div className="filter-row">
      <FilterField
        kind="language"
        label="Language"
        typed={state.options.language}
        choice={state.languages}
        control={filterControl(state, state.languages)}
        loaded={state.loaded}
        busy={busy}
        flagged={languageStale}
        onPick={(language) => pick({ language })}
        onOptions={onOptions}
        onError={onError}
      />
      <FilterField
        kind="scanlator"
        label="Scanlator"
        typed={state.options.scanlator}
        choice={state.scanlators}
        control={filterControl(state, state.scanlators)}
        loaded={state.loaded}
        busy={busy}
        flagged={scanlatorStale}
        onPick={(scanlator) => pick({ scanlator })}
        onOptions={onOptions}
        onError={onError}
      />
      {state.stale ? (
        <div className="stale-note" role="status">
          <AlertIcon />
          <span>Not applied yet</span>
          <button type="button" className="btn small warning" disabled={busy} onClick={onReload}>
            <RefreshIcon /> Reload
          </button>
        </div>
      ) : (
        !identified && <span className="filter-hint">Applied when the series loads</span>
      )}
    </div>
  );
}

interface FilterFieldProps {
  kind: FilterKind;
  label: string;
  /** The value typed (or last picked), shown until a load and by a site with no choice */
  typed: string;
  /** What the loaded series offers, null when the site lists none */
  choice: FilterChoice | null;
  control: FilterControl;
  /** Whether the series loaded: a failed load's typed value isn't an option */
  loaded: boolean;
  busy: boolean;
  flagged: boolean;
  onPick: (value: string) => void;
  onOptions: (patch: Partial<Options>) => Promise<void>;
  onError: (message: string) => void;
}

/** One filter: free text before a load, then a selector or a disabled field */
export function FilterField({
  kind,
  label,
  typed,
  choice,
  control,
  loaded,
  busy,
  flagged,
  onPick,
  onOptions,
  onError,
}: FilterFieldProps) {
  if (control === 'text') {
    return (
      <TextField
        className="inline-field"
        label={label}
        value={typed}
        placeholder="any"
        disabled={busy}
        flagged={flagged}
        onCommit={(value) => onOptions(kind === 'language' ? { language: value } : { scanlator: value })}
        onError={onError}
      />
    );
  }
  if (control === 'disabled' || choice === null) {
    // the site has no choice: nothing to show. The saved value stays, for
    // the sites that do take it.
    return null;
  }

  return (
    <SelectField
      className="inline-field"
      label={label}
      title={kind === 'language' ? 'The languages the site lists this series in; picking one reloads the list' : 'The scanlation groups the site lists for this series; picking one reloads the list'}
      value={shownSelection(choice, loaded)}
      options={filterOptions(choice, kind, 'en', loaded)}
      disabled={busy}
      onChange={onPick}
    />
  );
}
