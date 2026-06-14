function wdttBuildShareLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const userName = String(opts.remark || '').trim() || 'WDTT';
  const vpnName = String(opts.vpnTitle || opts.subTitle || opts.tag || opts.vpn || '').trim() || 'WDTT';
  const obj = {
    vpn: vpnName,
    name: userName,
    ip: host,
    dtls: Number(opts.dtls) || 56000,
    pass: password,
  };
  if (opts.deviceId) obj.did = opts.deviceId;
  obj.hash = wdttFormatVkHash(opts.vkHash, 'bare');
  if (opts.subUrl) obj.sub = opts.subUrl;
  return 'wdtt://' + Base64.encode(JSON.stringify(obj));
}

function wdttCopyLink(link, onOk, onErr) {
  if (!link) {
    if (onErr) onErr('Ссылка пуста');
    return;
  }
  ClipboardManager.copyText(link).then((ok) => {
    if (ok) {
      if (onOk) onOk();
    } else if (onErr) {
      onErr('Не удалось скопировать');
    }
  });
}

const WDTT_VK_HASH_PLACEHOLDER = 'VK_HASH';

/** Голый токен из хеша или ссылки VK (как stripVkUrl в клиентах). */
function wdttStripVkHashBare(raw) {
  let s = String(raw || '').trim();
  if (!s) return '';
  const prefixes = [
    'https://vk.com/call/join/', 'http://vk.com/call/join/',
    'https://m.vk.com/call/join/', 'http://m.vk.com/call/join/',
    'm.vk.com/call/join/', 'vk.com/call/join/',
    'https://vk.me/join/', 'http://vk.me/join/', 'vk.me/join/',
  ];
  const lower = s.toLowerCase();
  for (const p of prefixes) {
    if (lower.startsWith(p)) {
      s = s.slice(p.length);
      break;
    }
  }
  const q = s.indexOf('?');
  if (q !== -1) s = s.slice(0, q);
  const h = s.indexOf('#');
  if (h !== -1) s = s.slice(0, h);
  return s.replace(/\/+$/, '').trim();
}

/**
 * format:
 *   bare      — только токен (colon wdtt://, iOS/Android/desktop)
 *   join-url  — https://vk.com/call/join/TOKEN (qwdtt hashes=)
 * Пустое поле → всегда плейсхолдер VK_HASH.
 */
function wdttFormatVkHash(raw, format) {
  const bare = wdttStripVkHashBare(raw);
  if (!bare) return WDTT_VK_HASH_PLACEHOLDER;
  if (format === 'join-url') return 'https://vk.com/call/join/' + bare;
  return bare;
}

/** wdtt://IP:DTLS:WG:LOCAL:PASS:HASH[#name] — colon; HASH всегда в ссылке */
function wdttBuildColonLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const dtls = Number(opts.dtls) || 56000;
  const wg = Number(opts.wg) || 56001;
  const localPort = opts.localPort != null ? Number(opts.localPort) : 0;
  const name = String(opts.name || '').trim();
  const hash = wdttFormatVkHash(opts.vkHash, 'bare');
  let link = 'wdtt://' + host + ':' + dtls + ':' + wg + ':' + localPort + ':' + password + ':' + hash;
  if (name && opts.withName) link += '#' + name;
  return link;
}

/** qwdtt://config?... — hashes с полной ссылкой vk.com/call/join/ */
function wdttBuildQwdttLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const dtls = Number(opts.dtls) || 56000;
  const params = new URLSearchParams();
  params.set('name', String(opts.name || 'WDTT').trim() || 'WDTT');
  params.set('peer', host + ':' + dtls);
  params.set('hashes', wdttFormatVkHash(opts.vkHash, 'join-url'));
  params.set('workers', String(Number(opts.workers) || 18));
  params.set('port', String(Number(opts.port) || 9000));
  params.set('pass', password);
  return 'qwdtt://config?' + params.toString();
}

