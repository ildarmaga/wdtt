(function () {
  // Vue app for Subscription page
  const el = document.getElementById('subscription-data');
  if (!el) return;
  const textarea = document.getElementById('subscription-links');
  const rawLinks = (textarea?.value || '').split('\n').filter(Boolean);

  const data = {
    sId: el.getAttribute('data-sid') || '',
    subUrl: el.getAttribute('data-sub-url') || '',
    subJsonUrl: el.getAttribute('data-subjson-url') || '',
    subClashUrl: el.getAttribute('data-subclash-url') || '',
    download: el.getAttribute('data-download') || '',
    upload: el.getAttribute('data-upload') || '',
    used: el.getAttribute('data-used') || '',
    total: el.getAttribute('data-total') || '',
    remained: el.getAttribute('data-remained') || '',
    expireMs: (parseInt(el.getAttribute('data-expire') || '0', 10) || 0) * 1000,
    lastOnlineMs: (parseInt(el.getAttribute('data-lastonline') || '0', 10) || 0),
    downloadByte: parseInt(el.getAttribute('data-downloadbyte') || '0', 10) || 0,
    uploadByte: parseInt(el.getAttribute('data-uploadbyte') || '0', 10) || 0,
    totalByte: parseInt(el.getAttribute('data-totalbyte') || '0', 10) || 0,
    datepicker: el.getAttribute('data-datepicker') || 'gregorian',
  };

  // Normalize lastOnline to milliseconds if it looks like seconds
  if (data.lastOnlineMs && data.lastOnlineMs < 10_000_000_000) {
    data.lastOnlineMs *= 1000;
  }

  function copy(text) {
    ClipboardManager.copyText(text).then(ok => {
      const messageType = ok ? 'success' : 'error';
      Vue.prototype.$message[messageType](ok ? 'Copied' : 'Copy failed');
    });
  }

  function drawQR(value) {
    try {
      new QRious({ element: document.getElementById('qrcode'), value, size: 220 });
    } catch (e) {
      console.warn(e);
    }
  }

  // Try to extract a human label (email/ps) from different link types
  function linkName(link, idx) {
    try {
      if (link.startsWith('vmess://')) {
        const json = JSON.parse(atob(link.replace('vmess://', '')));
        if (json.ps) return json.ps;
        if (json.add && json.id) return json.add; // fallback host
      } else if (link.startsWith('vless://') || link.startsWith('trojan://')) {
        const hashIdx = link.indexOf('#');
        if (hashIdx !== -1) return decodeURIComponent(link.substring(hashIdx + 1));
        const qIdx = link.indexOf('?');
        if (qIdx !== -1) {
          const qs = new URL('http://x/?' + link.substring(qIdx + 1, hashIdx !== -1 ? hashIdx : undefined)).searchParams;
          if (qs.get('remark')) return qs.get('remark');
          if (qs.get('email')) return qs.get('email');
        }
        const at = link.indexOf('@');
        const protSep = link.indexOf('://');
        if (at !== -1 && protSep !== -1) return link.substring(protSep + 3, at);
      } else if (link.startsWith('ss://')) {
        const hashIdx = link.indexOf('#');
        if (hashIdx !== -1) return decodeURIComponent(link.substring(hashIdx + 1));
      } else if (link.startsWith('wdtt://')) {
        try {
          const json = JSON.parse(atob(link.replace('wdtt://', '')));
          if (json.ps) return json.ps;
          if (json.remark) return json.remark;
          if (json.email) return json.email;
          if (json.ip) return json.ip;
          if (json.add) return json.add;
        } catch (e) { /* ignore */ }
      }
    } catch (e) { /* ignore and fallback */ }
    return 'Link ' + (idx + 1);
  }

  const app = new Vue({
    delimiters: ['[[', ']]'],
    el: '#app',
    data: {
      themeSwitcher,
      app: data,
      links: rawLinks,
      lang: '',
    },
    async mounted() {
      this.lang = LanguageManager.getLanguage();
      const tpl = document.getElementById('subscription-data');
      const sj = tpl ? tpl.getAttribute('data-subjson-url') : '';
      const sc = tpl ? tpl.getAttribute('data-subclash-url') : '';
      if (sj) this.app.subJsonUrl = sj;
      if (sc) this.app.subClashUrl = sc;
      drawQR(this.app.subUrl);
      try {
        const elWdtt = document.getElementById('qrcode-wdtt');
        if (elWdtt && rawLinks[0]) {
          new QRious({ element: elWdtt, value: rawLinks[0], size: 220 });
        }
        const elJson = document.getElementById('qrcode-subjson');
        if (elJson && this.app.subJsonUrl) {
          new QRious({ element: elJson, value: this.app.subJsonUrl, size: 220 });
        }
        const elClash = document.getElementById('qrcode-subclash');
        if (elClash && this.app.subClashUrl) {
          new QRious({ element: elClash, value: this.app.subClashUrl, size: 220 });
        }
      } catch (e) { /* ignore */ }
    },
    computed: {
      isUnlimited() {
        return !this.app.totalByte;
      },
      isActive() {
        const now = Date.now();
        const expiryOk = !this.app.expireMs || this.app.expireMs >= now;
        const trafficOk = !this.app.totalByte || (this.app.uploadByte + this.app.downloadByte) <= this.app.totalByte;
        return expiryOk && trafficOk;
      },
    },
    methods: {
      copy,
      linkName,
      i18nLabel(key) {
        return '{{ i18n "' + key + '" }}';
      },
    },
  });
})();
