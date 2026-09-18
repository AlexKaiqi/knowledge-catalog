"use strict";
(() => {
  const raw = location.pathname.replace(/^\/repositories\//, "");
  const id = decodeURIComponent(raw);
  if (!id) { location.replace("/console"); return; }
  location.replace("/console#/repos/" + encodeURIComponent(id));
})();
