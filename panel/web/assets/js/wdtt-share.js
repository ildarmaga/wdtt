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
  if (opts.vkHash) obj.hash = opts.vkHash;
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
