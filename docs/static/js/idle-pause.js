// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

// Pause CSS animations while the tab is hidden. Browsers throttle
// requestAnimationFrame in background tabs but keep CSS animations
// painting, so the clouds, twinkling stars, and cursor block keep
// burning a tiny amount of GPU on a backgrounded micasa tab. Toggle
// a class on <html> and let CSS pause everything in one rule.

(() => {
  const html = document.documentElement;
  const sync = () => html.classList.toggle("animations-paused", document.hidden);
  document.addEventListener("visibilitychange", sync);
  sync();
})();
