function wdttCsrfToken() {
    const m = document.cookie.match(/(?:^|;\s*)wdtt-csrf=([^;]+)/);
    return m ? decodeURIComponent(m[1]) : '';
}

function wdttPostHeaders(extra) {
    const headers = Object.assign({
        'X-Requested-With': 'XMLHttpRequest',
        'X-CSRF-Token': wdttCsrfToken(),
    }, extra || {});
    return headers;
}

function wdttFetchPost(url, body, extraHeaders) {
    const headers = wdttPostHeaders(Object.assign({ 'Content-Type': 'application/json' }, extraHeaders || {}));
    return fetch(url, {
        method: 'POST',
        headers,
        body: typeof body === 'string' ? body : JSON.stringify(body),
    });
}
