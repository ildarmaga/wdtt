function wdttBuildShareLink(opts) {
  const host = String(opts.host || '').trim();
  const password = String(opts.password || '').trim();
  if (!host || !password) return '';
  const obj = {
    ps: opts.remark || 'WDTT',
    ip: host,
    dtls: Number(opts.dtls) || 56000,
    pass: password,
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
