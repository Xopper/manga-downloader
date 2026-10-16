// The search's site picker: a button that opens a list of the sites with a
// box to narrow it, since a plain <select> of some 460 sites can only be
// scrolled. It follows the combobox pattern: the box keeps the focus, the
// arrows move through the options, Enter picks, Escape closes.

import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { CheckIcon, ChevronIcon, SearchIcon } from './icons';
import { filterSiteGroups, type SiteGroup } from './search';

interface Props {
  groups: SiteGroup[];
  value: string;
  onChange: (value: string) => void;
}

export function SitePicker({ groups, value, onChange }: Props) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(0);
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);

  const current = groups.flatMap((g) => g.options).find((o) => o.value === value) ?? groups[0]?.options[0];
  const shown = useMemo(() => filterSiteGroups(groups, query), [groups, query]);
  const flat = useMemo(() => shown.flatMap((g) => g.options), [shown]);

  // a click outside closes it
  useEffect(() => {
    if (!open) {
      return;
    }
    const away = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', away);
    return () => document.removeEventListener('mousedown', away);
  }, [open]);

  // the box takes the focus as soon as it's there
  useEffect(() => {
    if (open) {
      input.current?.focus();
    }
  }, [open]);

  // the active option stays in view as the arrows move it
  useEffect(() => {
    if (open) {
      list.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView({ block: 'nearest' });
    }
  }, [open, active]);

  const show = () => {
    setQuery('');
    setActive(Math.max(0, groups.flatMap((g) => g.options).findIndex((o) => o.value === value)));
    setOpen(true);
  };
  const close = (refocus: boolean) => {
    setOpen(false);
    if (refocus) {
      button.current?.focus();
    }
  };
  const pick = (v: string) => {
    close(true);
    if (v !== value) {
      onChange(v);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        setActive((a) => Math.min(a + 1, flat.length - 1));
        break;
      case 'ArrowUp':
        e.preventDefault();
        setActive((a) => Math.max(a - 1, 0));
        break;
      case 'Home':
        e.preventDefault();
        setActive(0);
        break;
      case 'End':
        e.preventDefault();
        setActive(flat.length - 1);
        break;
      case 'Enter':
        e.preventDefault();
        if (flat[active]) {
          pick(flat[active].value);
        }
        break;
      case 'Escape':
        e.preventDefault();
        close(true);
        break;
      case 'Tab':
        close(false);
        break;
    }
  };

  let index = -1;
  return (
    <div className="site-picker" ref={root}>
      <button
        type="button"
        ref={button}
        className="input select-input search-site site-picker-button"
        aria-label={`Site: ${current?.label ?? 'All sites'}`}
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => (open ? close(false) : show())}
        onKeyDown={(e) => {
          if (!open && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) {
            e.preventDefault();
            show();
          }
        }}
      >
        <span className="site-picker-value">{current?.label ?? 'All sites'}</span>
        <ChevronIcon size={14} />
      </button>
      {open && (
        <div className="site-picker-pop">
          <label className="site-picker-find">
            <SearchIcon size={14} />
            <input
              ref={input}
              type="text"
              role="combobox"
              aria-label="Find a site"
              aria-expanded="true"
              aria-controls={`${id}-list`}
              aria-activedescendant={flat[active] ? `${id}-${active}` : undefined}
              aria-autocomplete="list"
              placeholder="Find a site…"
              spellCheck={false}
              autoComplete="off"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setActive(0);
              }}
              onKeyDown={onKeyDown}
            />
          </label>
          <div className="site-picker-list" role="listbox" id={`${id}-list`} aria-label="Sites" ref={list}>
            {flat.length === 0 && <p className="site-picker-empty">No site matches “{query.trim()}”</p>}
            {shown.map((g) => (
              <div key={g.label || 'all'} role="group" aria-label={g.label || undefined}>
                {g.label !== '' && (
                  <div className="site-picker-group" aria-hidden="true">
                    {g.label} · {g.options.length}
                  </div>
                )}
                {g.options.map((o) => {
                  index++;
                  const i = index;
                  const selected = o.value === value;
                  return (
                    <div
                      key={o.value || 'all'}
                      id={`${id}-${i}`}
                      data-index={i}
                      role="option"
                      aria-selected={selected}
                      className={`site-picker-option${i === active ? ' active' : ''}${selected ? ' on' : ''}`}
                      onMouseMove={() => i !== active && setActive(i)}
                      // mousedown, not click: the box must not lose the focus first
                      onMouseDown={(e) => {
                        e.preventDefault();
                        pick(o.value);
                      }}
                    >
                      <span className="site-picker-label">{o.label}</span>
                      {selected && <CheckIcon size={14} />}
                    </div>
                  );
                })}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
