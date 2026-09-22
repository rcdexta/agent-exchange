const announce = document.createElement('span');
announce.className = 'copy-status';
announce.setAttribute('role', 'status');
document.body.append(announce);

document.querySelectorAll('pre').forEach(pre => {
  const code = pre.querySelector('code');
  const guide = pre.dataset.copySource ? document.getElementById(pre.dataset.copySource) : null;
  if (!code || (pre.dataset.copySource && !guide)) return;
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'copy-button';
  const label = pre.dataset.copyLabel || '⧉';
  button.textContent = label;
  button.setAttribute('aria-label', pre.dataset.copyLabel || 'Copy code');
  button.title = 'Copy';
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(guide ? guide.value : code.textContent.trimEnd());
      if (guide) guide.hidden = true;
      button.textContent = pre.dataset.copyLabel ? 'Copied!' : '✓';
      button.dataset.state = 'copied';
      announce.textContent = 'Copied to clipboard.';
    } catch {
      if (guide) {
        guide.hidden = false;
        guide.focus();
        guide.select();
        announce.textContent = 'Could not copy automatically. Full guide selected; use your copy shortcut.';
      } else {
        const range = document.createRange();
        range.selectNodeContents(code);
        const selection = window.getSelection();
        selection.removeAllRanges();
        selection.addRange(range);
        announce.textContent = 'Could not copy automatically. Code selected; use your copy shortcut.';
      }
    }
    setTimeout(() => { button.textContent = label; delete button.dataset.state; announce.textContent = ''; }, 2400);
  });
  pre.append(button);
});

const exchange = document.querySelector('.exchange');
const replay = document.querySelector('[data-replay]');
const pause = document.querySelector('[data-pause]');
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');
if (exchange && replay && pause) {
  const frames = [...exchange.querySelectorAll('[data-at]')].sort((a, b) => +a.dataset.at - +b.dataset.at);
  const logs = [...exchange.querySelectorAll('.terminal-log')];
  let timer, next = 0, elapsed = 0, startedAt = 0, playing = false, started = false;

  function pauseExchange() {
    if (!playing) return;
    elapsed += performance.now() - startedAt;
    clearTimeout(timer);
    playing = false;
    pause.textContent = 'Resume';
  }

  function advance() {
    const position = elapsed + performance.now() - startedAt;
    const changed = new Set();
    while (next < frames.length && +frames[next].dataset.at <= position) {
      const frame = frames[next++];
      frame.hidden = false;
      changed.add(frame.closest('.terminal-log'));
    }
    changed.forEach(log => { log.scrollTop = log.scrollHeight; });
    if (next < frames.length) {
      timer = setTimeout(advance, +frames[next].dataset.at - position);
    } else {
      playing = false;
      pause.hidden = true;
    }
  }

  function resumeExchange() {
    startedAt = performance.now();
    playing = true;
    pause.hidden = false;
    pause.textContent = 'Pause';
    advance();
  }

  function playExchange() {
    if (reducedMotion.matches) return;
    clearTimeout(timer);
    started = true;
    next = elapsed = 0;
    frames.forEach(frame => { frame.hidden = true; });
    logs.forEach(log => { log.scrollTop = 0; });
    resumeExchange();
  }

  function motionPreference() {
    replay.hidden = reducedMotion.matches;
    if (reducedMotion.matches) {
      clearTimeout(timer);
      playing = false;
      pause.hidden = true;
      frames.forEach(frame => { frame.hidden = false; });
      logs.forEach(log => { log.scrollTop = 0; });
    }
  }

  replay.addEventListener('click', playExchange);
  pause.addEventListener('click', () => playing ? pauseExchange() : resumeExchange());
  // Reading back or leaving the demo pauses the finite replay, with one timer at most.
  logs.forEach(log => {
    log.addEventListener('wheel', pauseExchange, { passive: true });
    log.addEventListener('touchstart', pauseExchange, { passive: true });
    log.addEventListener('keydown', pauseExchange);
    log.addEventListener('pointerdown', pauseExchange);
  });
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) pauseExchange();
  });
  reducedMotion.addEventListener('change', motionPreference);
  motionPreference();
  if ('IntersectionObserver' in window) {
    const observer = new IntersectionObserver(entries => {
      const visible = entries.some(entry => entry.isIntersecting);
      if (visible && !started) playExchange();
      else if (!visible) pauseExchange();
    }, { threshold: 0.25 });
    observer.observe(exchange);
  }
}

const finder = document.querySelector('[data-page-finder]');
if (finder) {
  const menu = document.querySelector('.docs-menu');
  const desktop = matchMedia('(min-width: 1001px)');
  menu.open = desktop.matches;
  desktop.addEventListener('change', () => { menu.open = desktop.matches; });
  finder.closest('label').hidden = false;
  const links = [...document.querySelectorAll('[data-doc-link]')];
  finder.addEventListener('input', () => {
    const query = finder.value.toLowerCase().trim();
    links.forEach(link => { link.hidden = !link.textContent.toLowerCase().includes(query); });
    document.querySelectorAll('.docs-nav-group').forEach(group => {
      group.hidden = ![...group.querySelectorAll('a')].some(link => !link.hidden);
    });
    document.querySelector('.empty-search').hidden = links.some(link => !link.hidden);
  });
}
