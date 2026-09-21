"""Check the built public site's content and navigation contracts."""
import json
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parent
DIST = ROOT / 'dist'


class Page(HTMLParser):
    def __init__(self, text):
        super().__init__()
        self.ids, self.links, self.h1 = [], [], 0
        self.feed(text)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if 'id' in attrs:
            self.ids.append(attrs['id'])
        if tag == 'h1':
            self.h1 += 1
        if tag in ('a', 'link') and 'href' in attrs:
            self.links.append(attrs['href'])
        if tag in ('script', 'img') and 'src' in attrs:
            self.links.append(attrs['src'])


def route(path):
    path = unquote(path).lstrip('/')
    candidates = [DIST / path, DIST / (path + '.html'), DIST / path / 'index.html']
    return next((p for p in candidates if p.is_file()), None)


pages = {path: Page(path.read_text()) for path in DIST.rglob('*.html')}
checked = 0
for path, page in pages.items():
    assert page.h1 == 1, f'{path}: expected one H1'
    assert len(page.ids) == len(set(page.ids)), f'{path}: duplicate anchor IDs'
    assert '{{HEADER}}' not in path.read_text() and '{{FOOTER}}' not in path.read_text()
    for link in page.links:
        parsed = urlsplit(link)
        if parsed.scheme or parsed.netloc:
            continue
        target = route(parsed.path) if parsed.path else path
        assert target, f'{path}: broken link {link}'
        if parsed.fragment and target in pages:
            assert unquote(parsed.fragment) in pages[target].ids, f'{path}: broken anchor {link}'
        checked += 1

assert (DIST / 'agents.md').read_bytes() == (ROOT.parent / 'AGENTS.md').read_bytes()
for slug in ('pi', 'inbox', 'resource-safety', 'spawning'):
    assert '<strong>Unreleased.</strong>' not in (DIST / f'docs/{slug}.html').read_text()
assert 'text/markdown' in (DIST / '_headers').read_text()
assert '/AGENTS.md /agents.md 301' in (DIST / '_redirects').read_text()
assert 'Page not found' in (DIST / '404.html').read_text()
assert '/docs/pi' in (DIST / 'index.html').read_text()
assert 'source preview' not in (DIST / 'index.html').read_text()
assert 'ax pi --name worker' in (DIST / 'docs/pi.html').read_text()
print(json.dumps({'html_pages': len(pages), 'local_links_checked': checked, 'canonical_guide': 'identical'}))
