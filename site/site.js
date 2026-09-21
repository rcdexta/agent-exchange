const announce = document.createElement('span');
announce.className = 'copy-status';
announce.setAttribute('role', 'status');
document.body.append(announce);

document.querySelectorAll('pre').forEach(pre => {
  const code = pre.querySelector('code');
  if (!code) return;
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'copy-button';
  const label = pre.dataset.copyLabel || '⧉';
  button.textContent = label;
  button.setAttribute('aria-label', pre.dataset.copyLabel || 'Copy code');
  button.title = 'Copy';
  button.addEventListener('click', async () => {
    try {
      await navigator.clipboard.writeText(code.textContent.trimEnd());
      button.textContent = pre.dataset.copyLabel ? 'Copied!' : '✓';
      button.dataset.state = 'copied';
      announce.textContent = 'Copied to clipboard.';
    } catch {
      const range = document.createRange();
      range.selectNodeContents(code);
      const selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
      announce.textContent = 'Could not copy automatically. Code selected; use your copy shortcut.';
    }
    setTimeout(() => { button.textContent = label; delete button.dataset.state; announce.textContent = ''; }, 2400);
  });
  pre.append(button);
});

const exchange = document.querySelector('.exchange');
const replay = document.querySelector('[data-replay]');
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');
function playExchange() {
  if (reducedMotion.matches) return;
  exchange.classList.remove('is-playing');
  requestAnimationFrame(() => requestAnimationFrame(() => exchange.classList.add('is-playing')));
}
if (exchange && replay) {
  replay.hidden = reducedMotion.matches;
  replay.addEventListener('click', playExchange);
  reducedMotion.addEventListener('change', () => {
    replay.hidden = reducedMotion.matches;
    if (reducedMotion.matches) exchange.classList.remove('is-playing');
  });
  playExchange();
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
