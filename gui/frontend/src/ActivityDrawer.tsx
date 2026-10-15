// The run's details, as a drawer over the window: the Activity panel (the
// summary and one row per chapter), opened from the progress strip.

import { ActivityPanel } from './ActivityPanel';
import { XIcon } from './icons';
import type { State } from './types';

interface Props {
  state: State;
  onOpenFolder: () => void;
  onClose: () => void;
}

export function ActivityDrawer({ state, onOpenFolder, onClose }: Props) {
  return (
    <section
      className="log-drawer drawer activity-drawer"
      aria-label="Download details"
      onKeyDown={(e) => {
        if (e.key === 'Escape') {
          onClose();
        }
      }}
    >
      <div className="log-head">
        <h2>{state.running ? 'This download' : 'Last download'}</h2>
        <button type="button" className="btn ghost small icon-only" aria-label="Close the details" onClick={onClose} autoFocus>
          <XIcon size={14} />
        </button>
      </div>
      <div className="drawer-body activity-body">
        <ActivityPanel state={state} onOpenFolder={onOpenFolder} />
      </div>
    </section>
  );
}
