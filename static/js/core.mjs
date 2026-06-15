// core.mjs — the tiny island loader + HTMX CSRF wiring. No framework, no bundler.
//
// Loaded as a module from 'self'. The import map (carrying the per-request CSP
// nonce) resolves bare "@islands/<name>" specifiers to the fingerprinted module
// URLs, so islands are loaded lazily and only on pages that request them.
//
// Everything here runs in an external module, so the strict, no-unsafe-inline
// CSP is preserved — there are no inline event handlers anywhere in the app.

// 1) Attach the CSRF token to every HTMX request as a header. The token is read
//    from the server-rendered <meta name="csrf-token"> — never from the cookie,
//    which is HttpOnly. See internal/core/httpx/csrf.go for the verification side.
const csrfToken = document.querySelector('meta[name="csrf-token"]')?.content;
document.body.addEventListener("htmx:configRequest", (event) => {
  if (csrfToken) {
    event.detail.headers["X-CSRF-Token"] = csrfToken;
  }
});

// 2) Island loader. Any element with data-island="<name>" is mounted by importing
//    @islands/<name> and calling its exported mount(el, opts). Per-element options
//    come from data-opt-* attributes (data-opt-url="…" -> opts.url).
async function mountIslands(root = document) {
  const elements = root.querySelectorAll("[data-island]:not([data-island-mounted])");
  for (const el of elements) {
    const name = el.dataset.island;
    el.setAttribute("data-island-mounted", "");
    try {
      const module = await import(`@islands/${name}`);
      if (typeof module.mount === "function") {
        module.mount(el, optionsFrom(el));
      } else {
        console.error(`island "${name}" exports no mount() function`);
      }
    } catch (err) {
      console.error(`failed to load island "${name}"`, err);
    }
  }
}

function optionsFrom(el) {
  const opts = {};
  for (const [key, value] of Object.entries(el.dataset)) {
    if (key.startsWith("opt") && key.length > 3) {
      const name = key[3].toLowerCase() + key.slice(4);
      opts[name] = value;
    }
  }
  return opts;
}

// Modules are deferred, so the DOM is parsed by the time this runs. Mount once
// now, then again after each HTMX swap so islands inside swapped markup activate.
mountIslands();
document.body.addEventListener("htmx:afterSwap", (event) => mountIslands(event.target));
