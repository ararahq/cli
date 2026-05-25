const REPO = "ararahq/cli";
const SH_URL = `https://raw.githubusercontent.com/${REPO}/main/install.sh`;
const PS1_URL = `https://raw.githubusercontent.com/${REPO}/main/install.ps1`;
const DOCS_URL = "https://docs.ararahq.com/cli";

const SH_HEADERS = { "Content-Type": "text/x-shellscript; charset=utf-8" };
const PS1_HEADERS = { "Content-Type": "text/plain; charset=utf-8" };
const CACHE_HEADER = { "Cache-Control": "public, max-age=300" };

export default {
  async fetch(request) {
    const url = new URL(request.url);
    const ua = (request.headers.get("User-Agent") || "").toLowerCase();
    const looksLikeShell = /curl|wget|fetch/.test(ua);

    if (url.pathname === "/install-cli" || url.pathname === "/install-cli.sh") {
      return proxy(SH_URL, SH_HEADERS);
    }

    if (url.pathname === "/install-cli.ps1") {
      return proxy(PS1_URL, PS1_HEADERS);
    }

    // Root: serve install.sh when called by curl/wget (so `curl ... | sh` works),
    // otherwise redirect humans to the docs.
    if (url.pathname === "/" || url.pathname === "") {
      if (looksLikeShell) {
        return proxy(SH_URL, SH_HEADERS);
      }
      return Response.redirect(DOCS_URL, 302);
    }

    return new Response("Not found. Try /install-cli or /install-cli.ps1\n", {
      status: 404,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  },
};

async function proxy(upstream, contentHeaders) {
  const upstreamResponse = await fetch(upstream, {
    cf: { cacheTtl: 300, cacheEverything: true },
  });

  if (!upstreamResponse.ok) {
    return new Response(`Upstream fetch failed (${upstreamResponse.status})\n`, {
      status: 502,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }

  return new Response(upstreamResponse.body, {
    status: 200,
    headers: { ...contentHeaders, ...CACHE_HEADER },
  });
}
