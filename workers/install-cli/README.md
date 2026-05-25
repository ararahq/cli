# arara-install-cli (Cloudflare Worker)

Serve os scripts de instalação do AraraHQ CLI em `get.ararahq.com`.

```
GET https://get.ararahq.com/install-cli      → install.sh   (Unix)
GET https://get.ararahq.com/install-cli.ps1  → install.ps1  (Windows)
GET https://get.ararahq.com/                 → 302 docs.ararahq.com/cli
```

Os scripts são puxados de `raw.githubusercontent.com/ararahq/cli/main/`. Cache de 5min na borda da Cloudflare.

## Deploy

1. **Instale o Wrangler** (uma vez): `npm i -g wrangler`
2. **Login**: `wrangler login`
3. **Deploy**: dentro deste diretório, `wrangler deploy`
4. **Custom domain**: dashboard Cloudflare → Workers → `arara-install-cli` → Settings → Triggers → Add Custom Domain → `get.ararahq.com`

DNS é criado automaticamente quando você adiciona o custom domain (Cloudflare resolve internamente).

## Testar antes do deploy

```bash
wrangler dev
curl http://127.0.0.1:8787/install-cli
```

## Validar produção

```bash
curl -fsSL https://get.ararahq.com/install-cli | head -20
curl -I https://get.ararahq.com/install-cli.ps1
```
