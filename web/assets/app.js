// ant console behavior: theme toggle, the mobile nav drawer, and select
// auto-submit. Progressive enhancement only — every page works without it
// (8000_ant_serve §3, §6.3). No dependencies.
(function () {
  "use strict";

  // Theme toggle, persisted in localStorage (the inline head script applies it
  // before first paint to avoid a flash).
  var root = document.documentElement;
  document.querySelectorAll("[data-theme-toggle]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var next = root.dataset.theme === "dark" ? "light" : "dark";
      root.dataset.theme = next;
      try { localStorage.setItem("ant-theme", next); } catch (e) {}
    });
  });

  // Mobile navigation drawer.
  document.querySelectorAll("[data-menu-toggle]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      document.body.classList.toggle("menu-open");
    });
  });
  document.addEventListener("click", function (e) {
    if (!document.body.classList.contains("menu-open")) return;
    var sidebar = document.querySelector(".sidebar");
    var toggle = e.target.closest("[data-menu-toggle]");
    if (toggle) return;
    if (sidebar && !sidebar.contains(e.target)) document.body.classList.remove("menu-open");
  });

  // Selects that submit their form on change (graph depth, etc).
  document.querySelectorAll("select[data-autosubmit]").forEach(function (sel) {
    sel.addEventListener("change", function () {
      if (sel.form) sel.form.submit();
    });
  });

  // "/" focuses the omni bar, like a command palette shortcut.
  document.addEventListener("keydown", function (e) {
    if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
    var tag = (e.target.tagName || "").toLowerCase();
    if (tag === "input" || tag === "select" || tag === "textarea") return;
    var omni = document.querySelector(".omni-input");
    if (omni) { e.preventDefault(); omni.focus(); }
  });
})();
