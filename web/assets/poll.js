// poll.js drives the loading screen. While a record, collection, or graph is
// being fetched in the background, it asks the status endpoint named in
// #fetch[data-status] every second and reloads the page the instant the work is
// ready (or failed, to surface the error). Loaded only on the loading page
// (8000_ant_serve §24). With JavaScript off, the page's <noscript> meta refresh
// does the same job, more coarsely. No dependencies.
(function () {
  "use strict";
  var el = document.getElementById("fetch");
  if (!el) return;
  var url = el.getAttribute("data-status");
  if (!url) return;

  var tries = 0;

  function poll() {
    fetch(url, { headers: { "Accept": "application/json" }, cache: "no-store" })
      .then(function (r) { return r.json(); })
      .then(function (s) {
        if (s && (s.status === "ready" || s.status === "error")) {
          location.reload();
          return;
        }
        schedule();
      })
      .catch(schedule);
  }

  function schedule() {
    tries++;
    // Gentle backoff: poll every second for the first ~30s, then every two.
    setTimeout(poll, tries > 30 ? 2000 : 1000);
  }

  schedule();
})();
