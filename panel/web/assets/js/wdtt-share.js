function wdttBuildShareLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const obj = {
    v: '1',
    ps: opts.remark || 'WDTT',
    tag: opts.tag || 'wdtt-in',
    add: host,
    dtls: Number(opts.dtls) || 56000,
    wg: Number(opts.wg) || 56001,
    lp: Number(opts.clientPort) || 9000,
    id: password,
  };
  if (opts.deviceId) obj.did = opts.deviceId;
  if (opts.vkHash) obj.hash = opts.vkHash;
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
