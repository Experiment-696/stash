(() => {
  if (!window.PluginApi) return;

  // CS-01 moved navigation and routes into the internal trusted-extension
  // registry. This development package intentionally does not register them
  // again, which prevents duplicate menu items and preserves the P1A rule that
  // arbitrary plugin JavaScript remains Admin-only.
  window.console.info(
    "Cam Shows Lite: trusted navigation and routes are supplied by the bundled application."
  );
})();
