import { Environment, WindowIsFullscreen, WindowToggleMaximise } from '../wailsjs/runtime/runtime';

// Things a double click on the header leaves alone: they have their own use
const OWN_CLICKS = 'button, input, select, textarea, a, form, [role]';

// On macOS the window has no title bar (main.go), so the header stands in
// for it: html gets .mac-frameless, which makes the header drag the window
// and starts its content below the window buttons. Those buttons are gone in
// full screen, so .fullscreen gives the room back. A double click on an
// empty part of the header zooms the window, as on a title bar.
export function setUpWindowFrame(): void {
  if ((window as { runtime?: unknown }).runtime === undefined) return;
  const root = document.documentElement;
  void Environment().then((env) => {
    if (env.platform !== 'darwin') return;
    root.classList.add('mac-frameless');
    const checkFullscreen = (): void => {
      void WindowIsFullscreen().then((full) => root.classList.toggle('fullscreen', full));
    };
    checkFullscreen();
    window.addEventListener('resize', checkFullscreen);
    document.addEventListener('dblclick', (e) => {
      const target = e.target as Element | null;
      if (target?.closest('.topbar') && !target.closest(OWN_CLICKS)) WindowToggleMaximise();
    });
  });
}
