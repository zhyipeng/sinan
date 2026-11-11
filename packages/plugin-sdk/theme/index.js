// Theme snapshots contain presentation data only; no bridge call is required.
export function currentTheme(){return window.__sinanTheme?JSON.parse(JSON.stringify(window.__sinanTheme)):undefined;}
export function onThemeChanged(listener){const callback=()=>listener(currentTheme());window.addEventListener('sinan:theme-changed',callback);return()=>window.removeEventListener('sinan:theme-changed',callback);}
