// Runs before the first paint so a saved dark theme never flashes light. It is a
// separate file because the server's Content-Security-Policy forbids inline scripts.
try {
  var saved = localStorage.getItem('receh-theme');
  if (saved === 'dark' || saved === 'light') document.documentElement.dataset.theme = saved;
} catch (e) {
  /* Storage may be blocked; the system preference still applies. */
}
