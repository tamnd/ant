// graph.js renders the link graph the /graph page embeds as JSON in
// #graph[data-payload]. It is a tiny self-contained force layout drawn to SVG;
// nodes link back to their record. Loaded only on the graph page
// (8000_ant_serve §10). No dependencies.
(function () {
  "use strict";
  var host = document.getElementById("graph");
  if (!host) return;

  var data;
  try { data = JSON.parse(host.dataset.payload || "{}"); } catch (e) { return; }
  var nodes = (data.nodes || []).map(function (n) { return Object.assign({}, n); });
  var edges = data.edges || [];
  if (!nodes.length) { host.innerHTML = '<p class="graph-noscript muted">No nodes to draw.</p>'; return; }

  var W = host.clientWidth || 720, H = host.clientHeight || 460;
  var byURI = {};
  nodes.forEach(function (n, i) {
    byURI[n.uri] = n;
    // Seed on a circle so the layout opens up rather than collapsing.
    var a = (i / nodes.length) * Math.PI * 2;
    n.x = W / 2 + Math.cos(a) * Math.min(W, H) * 0.3;
    n.y = H / 2 + Math.sin(a) * Math.min(W, H) * 0.3;
    n.vx = 0; n.vy = 0;
    n.root = n.uri === data.root;
  });
  var links = edges.filter(function (e) { return byURI[e.from] && byURI[e.to]; })
    .map(function (e) { return { s: byURI[e.from], t: byURI[e.to] }; });

  // Force-directed layout: Coulomb repulsion, Hooke springs on edges, and a mild
  // pull to center. A fixed iteration count keeps it cheap and deterministic.
  var K = 130;
  for (var iter = 0; iter < 320; iter++) {
    for (var i = 0; i < nodes.length; i++) {
      var a = nodes[i];
      for (var j = i + 1; j < nodes.length; j++) {
        var b = nodes[j];
        var dx = a.x - b.x, dy = a.y - b.y;
        var d2 = dx * dx + dy * dy || 0.01;
        var d = Math.sqrt(d2);
        var rep = (K * K) / d2;
        var fx = (dx / d) * rep, fy = (dy / d) * rep;
        a.vx += fx; a.vy += fy; b.vx -= fx; b.vy -= fy;
      }
      a.vx += (W / 2 - a.x) * 0.01;
      a.vy += (H / 2 - a.y) * 0.01;
    }
    links.forEach(function (l) {
      var dx = l.t.x - l.s.x, dy = l.t.y - l.s.y;
      var d = Math.sqrt(dx * dx + dy * dy) || 0.01;
      var f = (d - K) * 0.04;
      var fx = (dx / d) * f, fy = (dy / d) * f;
      l.s.vx += fx; l.s.vy += fy; l.t.vx -= fx; l.t.vy -= fy;
    });
    var damp = 0.85;
    nodes.forEach(function (n) {
      n.x += n.vx * 0.04; n.y += n.vy * 0.04;
      n.vx *= damp; n.vy *= damp;
      n.x = Math.max(24, Math.min(W - 24, n.x));
      n.y = Math.max(24, Math.min(H - 24, n.y));
    });
  }

  var svgNS = "http://www.w3.org/2000/svg";
  var svg = document.createElementNS(svgNS, "svg");
  svg.setAttribute("viewBox", "0 0 " + W + " " + H);

  links.forEach(function (l) {
    var line = document.createElementNS(svgNS, "line");
    line.setAttribute("x1", l.s.x); line.setAttribute("y1", l.s.y);
    line.setAttribute("x2", l.t.x); line.setAttribute("y2", l.t.y);
    line.setAttribute("stroke", "currentColor");
    line.setAttribute("stroke-opacity", "0.18");
    line.setAttribute("stroke-width", "1.4");
    svg.appendChild(line);
  });

  nodes.forEach(function (n) {
    var g = document.createElementNS(svgNS, "a");
    g.setAttributeNS("http://www.w3.org/1999/xlink", "href", "/view?uri=" + encodeURIComponent(n.uri));
    g.setAttribute("href", "/view?uri=" + encodeURIComponent(n.uri));

    var c = document.createElementNS(svgNS, "circle");
    c.setAttribute("cx", n.x); c.setAttribute("cy", n.y);
    c.setAttribute("r", n.root ? 9 : 6);
    c.setAttribute("fill", n.accent || "#888");
    c.setAttribute("stroke", "hsl(var(--bg))");
    c.setAttribute("stroke-width", "2");
    g.appendChild(c);

    var t = document.createElementNS(svgNS, "text");
    t.setAttribute("x", n.x + 10); t.setAttribute("y", n.y + 4);
    t.setAttribute("font-size", n.root ? "13" : "12");
    t.setAttribute("fill", "currentColor");
    t.textContent = (n.label || n.uri).slice(0, 28);
    g.appendChild(t);

    var title = document.createElementNS(svgNS, "title");
    title.textContent = n.uri;
    g.appendChild(title);

    svg.appendChild(g);
  });

  host.textContent = "";
  host.appendChild(svg);
})();
