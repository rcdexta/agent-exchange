# useax.dev

The site serves the repository's `AGENTS.md` at `https://useax.dev/agents.md`.
The build copies that file byte for byte; edit the root guide, not the generated
copy. `site/dist` is generated and ignored by Git. The uppercase URL redirects
to the lowercase public URL.

Cloudflare Workers serves static assets without a request handler or database.
Deploy only to the personal **rcdexta** account after it owns `useax.dev`:

```sh
cd site
npm ci
npx wrangler whoami
npm run deploy
```

When several Cloudflare accounts are available, set `CLOUDFLARE_ACCOUNT_ID` to the
verified rcdexta account ID before deployment. Browser login and Wrangler login
are separate; use `npx wrangler login` if the CLI is not authenticated.

After deployment, verify that the public guide returns HTTP 200, a Markdown
content type, and the same bytes as `AGENTS.md`. Check the root page and uppercase
redirect as well. Every guide change needs a fresh build and deployment.
