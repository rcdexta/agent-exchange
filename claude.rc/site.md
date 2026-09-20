# useax.dev

The site serves the landing page, documentation at `https://useax.dev/docs`, and
the repository's `AGENTS.md` at `https://useax.dev/agents.md`. The build copies the
agent guide byte for byte and renders the human installation page from the same
source. The uppercase URL redirects to the lowercase public URL.

Documentation sources live in `claude.rc`, with titles, routes, and navigation
defined in `site/pages.mjs`. Add a Markdown file and a page entry to publish a new
guide. Relative links to other published guides become local documentation links;
other repository links point to GitHub. Raw HTML in Markdown is disabled.

`site/dist` is generated and ignored by Git. Edit the sources, not that directory.
The site ships static HTML and CSS, plus a small script for copying code, replaying
the illustrative exchange, and filtering documentation page titles. Navigation and
all documentation content work without JavaScript. The animation runs once and
respects reduced-motion preferences.

Cloudflare Workers serves static assets without a request handler or database.
Deploy only to the personal **rcdexta** account after it owns `useax.dev`:

```sh
cd site
npm ci
npm run build
npm test
npx wrangler whoami
npm run deploy
```

When several Cloudflare accounts are available, set `CLOUDFLARE_ACCOUNT_ID` to the
verified rcdexta account ID before deployment. Browser login and Wrangler login
are separate; use `npx wrangler login` if the CLI is not authenticated.

After deployment, verify that the public guide returns HTTP 200, a Markdown
content type, and the same bytes as `AGENTS.md`. Check the root page and uppercase
redirect as well. Every guide change needs a fresh build and deployment.

For a local preview, run `npm run build` and `npx wrangler dev` from `site`.
Check the landing page and docs at desktop and mobile widths. The site tests check
rendered links, heading anchors, release labels, the custom 404 page, and the
byte-for-byte installation guide.
