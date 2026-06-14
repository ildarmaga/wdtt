package main

import (
	"net"
	"sync"
)

var wrapAuthByAddr sync.Map

func registerWrapAuth(addr net.Addr, password string) {
	if addr == nil || password == "" {
		return
	}
	wrapAuthByAddr.Store(addr.String(), password)
}

func lookupWrapAuth(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	if v, ok := wrapAuthByAddr.Load(addr.String()); ok {
		return v.(string)
	}
	return ""
}

func clearWrapAuth(addr net.Addr) {
	if addr != nil {
		wrapAuthByAddr.Delete(addr.String())
	}
}
