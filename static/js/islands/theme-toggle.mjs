// theme-toggle island — native-only (localStorage + a `.dark` class on <html>),
// zero third-party dependencies. The behavior is encapsulated behind mount(el),
// so swapping in a different implementation touches only this file.
//
// Mounted by core.mjs when an element has data-island="theme-toggle".

const STORAGE_KEY = "theme"; // "dark" | "light"

function preferred() {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === "dark" || saved === "light") return saved;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function apply(theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

/**
 * mount wires the toggle button. opts is unused here but is the standard island
 * signature (core.mjs passes data-opt-* attributes as opts).
 * @param {HTMLElement} el
 */
export function mount(el) {
  apply(preferred());
  el.addEventListener("click", () => {
    const next = document.documentElement.classList.contains("dark") ? "light" : "dark";
    localStorage.setItem(STORAGE_KEY, next);
    apply(next);
  });
}
