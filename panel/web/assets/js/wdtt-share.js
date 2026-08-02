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
  if (opts.subUrl) obj.sub = opts.subUrl;
  if (typeof wdttFormatVkHashes === 'function') {
    obj.hash = wdttFormatVkHashes(opts.vkHash, 'bare');
  } else if (opts.vkHash) {
    obj.hash = String(opts.vkHash).trim();
  }
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
const WDTT_VK_HASH_MAX = 4;

/** Голый токен из хеша или ссылки VK (как stripVkUrl в клиентах). */
function wdttStripVkHashBare(raw) {
  let s = String(raw || '').trim();
  if (!s) return '';
  const lower = s.toLowerCase();
  const joinIdx = lower.indexOf('/call/join/');
  if (joinIdx >= 0) {
    s = s.slice(joinIdx + '/call/join/'.length);
  } else if (lower.startsWith('http://') || lower.startsWith('https://')) {
    return '';
  } else {
    const prefixes = [
      'https://vk.ru/call/join/', 'http://vk.ru/call/join/',
      'https://vk.com/call/join/', 'http://vk.com/call/join/',
      'https://m.vk.ru/call/join/', 'http://m.vk.ru/call/join/',
      'https://m.vk.com/call/join/', 'http://m.vk.com/call/join/',
      'm.vk.ru/call/join/', 'vk.ru/call/join/',
      'm.vk.com/call/join/', 'vk.com/call/join/',
      'https://vk.me/join/', 'http://vk.me/join/', 'vk.me/join/',
    ];
    for (const p of prefixes) {
      if (lower.startsWith(p)) {
        s = s.slice(p.length);
        break;
      }
    }
  }
  const q = s.indexOf('?');
  if (q !== -1) s = s.slice(0, q);
  const h = s.indexOf('#');
  if (h !== -1) s = s.slice(0, h);
  const slash = s.indexOf('/');
  if (slash !== -1) s = s.slice(0, slash);
  return s.replace(/\/+$/, '').trim();
}

/** Список bare-хешей (до 4), разделители: запятая, пробел, перевод строки. */
function wdttParseVkHashes(raw) {
  const s = String(raw || '').trim();
  if (!s) return [];
  const seen = new Set();
  const out = [];
  for (const part of s.split(/[,;\n\r\t ]+/)) {
    const bare = wdttStripVkHashBare(part);
    if (!bare || seen.has(bare)) continue;
    seen.add(bare);
    out.push(bare);
    if (out.length >= WDTT_VK_HASH_MAX) break;
  }
  return out;
}

/**
 * format:
 *   bare      — bare-токены через запятую (colon wdtt://, qwdtt hashes=)
 *   join-url  — https://vk.ru/call/join/TOKEN (по одному на хеш, через запятую)
 * limit — макс. число хешей (iOS = 1)
 * Пустое поле → плейсхолдер VK_HASH.
 */
function wdttFormatVkHashes(raw, format, limit) {
  let list = wdttParseVkHashes(raw);
  if (typeof limit === 'number' && limit > 0) list = list.slice(0, limit);
  if (!list.length) return WDTT_VK_HASH_PLACEHOLDER;
  if (format === 'join-url') {
    return list.map((h) => 'https://vk.ru/call/join/' + h).join(',');
  }
  return list.join(',');
}

/** @deprecated используйте wdttFormatVkHashes */
function wdttFormatVkHash(raw, format) {
  return wdttFormatVkHashes(raw, format);
}

/** wdtt://IP:DTLS:WG:LOCAL:PASS:HASH[,HASH…][#name] — colon */
function wdttBuildColonLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const dtls = Number(opts.dtls) || 56000;
  const wg = Number(opts.wg) || 56001;
  const localPort = opts.localPort != null ? Number(opts.localPort) : 0;
  const name = String(opts.name || '').trim();
  const hash = wdttFormatVkHashes(opts.vkHash, 'bare', opts.hashLimit);
  let link = 'wdtt://' + host + ':' + dtls + ':' + wg + ':' + localPort + ':' + password + ':' + hash;
  if (name && opts.withName) link += '#' + name;
  return link;
}

/** qwdtt://config?... — hashes: bare-токены через запятую (как в qWDTT) */
function wdttBuildQwdttLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const params = new URLSearchParams();
  params.set('name', String(opts.name || 'WDTT').trim() || 'WDTT');
  params.set('peer', host);
  params.set('hashes', wdttFormatVkHashes(opts.vkHash, 'bare'));
  params.set('workers', String(Number(opts.workers) || 18));
  params.set('port', String(Number(opts.port) || 9000));
  return 'qwdtt://config?' + params.toString() + '&pass=' + password;
}
