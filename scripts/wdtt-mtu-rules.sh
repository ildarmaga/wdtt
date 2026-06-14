#!/bin/bash
# WDTT MTU rules: TCP MSS clamp + DF-clear for 10.66.66.0/24
# Единый источник — wdtt-install, deploy.sh, wdtt-xray-rules.sh (только вызывает up).
set +e

IFACE="${WDTT_IFACE:-wdtt0}"
TUN_NET="${WDTT_TUN_NET:-10.66.66.0/24}"
IPT_COMMENT="${WDTT_IPT_COMMENT:-WDTT_MANAGED}"
DEFAULT_MTU="${WDTT_DEFAULT_MTU:-1280}"

detect_wan() {
    ip route get 8.8.8.8 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -1
}

tun_mtu() {
    local m
    m="$(cat "/sys/class/net/${IFACE}/mtu" 2>/dev/null)"
    [[ -n "$m" ]] && echo "$m" || echo "$DEFAULT_MTU"
}

apply_mss_clamp() {
    local subnet="$1"
    if command -v iptables >/dev/null 2>&1; then
        iptables -t mangle -C FORWARD -s "$subnet" -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment "$IPT_COMMENT" -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || \
            iptables -t mangle -I FORWARD -s "$subnet" -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment "$IPT_COMMENT" -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || true
        iptables -t mangle -C FORWARD -d "$subnet" -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment "$IPT_COMMENT" -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || \
            iptables -t mangle -I FORWARD -d "$subnet" -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment "$IPT_COMMENT" -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || true
    fi
    if command -v nft >/dev/null 2>&1; then
        nft delete table inet wdtt_mangle 2>/dev/null
        nft -f - <<NFT
table inet wdtt_mangle {
    chain forward {
        type filter hook forward priority -150; policy accept;
        ip saddr ${subnet} tcp flags syn tcp option maxseg size set rt mtu
        ip daddr ${subnet} tcp flags syn tcp option maxseg size set rt mtu
    }
}
NFT
    fi
}

apply_df_clear() {
    local wan="$1" mtu="$2"
    [[ -z "$wan" ]] && return 0
    if command -v nft >/dev/null 2>&1; then
        nft delete table ip wdtt_mtu 2>/dev/null
        nft -f - <<NFT
table ip wdtt_mtu {
    chain clampdf {
        type filter hook prerouting priority -90; policy accept;
        iifname "${wan}" ip daddr ${TUN_NET} ip length gt ${mtu} ip frag-off & 0x4000 == 0x4000 ip frag-off set 0
    }
}
NFT
    fi
}

cleanup_mtu_rules() {
    if command -v iptables >/dev/null 2>&1; then
        while iptables -t mangle -D FORWARD -s "$TUN_NET" -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment "$IPT_COMMENT" -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null; do :; done
        while iptables -t mangle -D FORWARD -d "$TUN_NET" -p tcp -m tcp --tcp-flags SYN,RST SYN -m comment --comment "$IPT_COMMENT" -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null; do :; done
    fi
    nft delete table inet wdtt_mangle 2>/dev/null
    nft delete table ip wdtt_mtu 2>/dev/null
}

case "${1:-up}" in
down)
    cleanup_mtu_rules
    echo "wdtt-mtu rules removed"
    exit 0
    ;;
up)
    WAN="$(detect_wan)"
    MTU="$(tun_mtu)"
    apply_mss_clamp "$TUN_NET"
    apply_df_clear "$WAN" "$MTU"
    echo "wdtt-mtu rules applied (MSS ${TUN_NET}, DF-clear ${WAN:-?}->${TUN_NET} >${MTU}B)"
    exit 0
    ;;
*)
    echo "Usage: $0 up|down" >&2
    exit 1
    ;;
esac
