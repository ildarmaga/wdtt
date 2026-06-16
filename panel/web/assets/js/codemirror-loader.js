// Lazy-load CodeMirror + plugins (xray page only).
const CodeMirrorLoader = (function () {
  let loadPromise = null;

  function assetBase() {
    return (typeof basePath === 'string' ? basePath : '/') + 'assets/codemirror/';
  }

  function loadCSS(href) {
    return new Promise((resolve, reject) => {
      const link = document.createElement('link');
      link.rel = 'stylesheet';
      link.href = href;
      link.onload = () => resolve();
      link.onerror = () => reject(new Error('css load failed: ' + href));
      document.head.appendChild(link);
    });
  }

  function loadScript(src) {
    return new Promise((resolve, reject) => {
      const script = document.createElement('script');
      script.src = src;
      script.async = false;
      script.onload = () => resolve();
      script.onerror = () => reject(new Error('script load failed: ' + src));
      document.body.appendChild(script);
    });
  }

  function load() {
    if (window.CodeMirror) {
      return Promise.resolve();
    }
    if (loadPromise) {
      return loadPromise;
    }
    const base = assetBase();
    const ver = typeof curVer === 'string' && curVer ? '?' + curVer : '';
    loadPromise = (async () => {
      await loadCSS(base + 'codemirror.min.css' + ver);
      await loadCSS(base + 'fold/foldgutter.css');
      await loadCSS(base + 'xq.min.css' + ver);
      await loadCSS(base + 'lint/lint.css');
      const scripts = [
        'codemirror.min.js',
        'javascript.js',
        'jshint.js',
        'jsonlint.js',
        'lint/lint.js',
        'lint/javascript-lint.js',
        'hint/javascript-hint.js',
        'fold/foldcode.js',
        'fold/foldgutter.js',
        'fold/brace-fold.js',
      ];
      for (const name of scripts) {
        const q = name.endsWith('.min.js') ? ver : '';
        await loadScript(base + name + q);
      }
    })();
    return loadPromise;
  }

  return { load };
})();
