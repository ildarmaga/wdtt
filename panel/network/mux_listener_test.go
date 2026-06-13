package network

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestMuxListenerDoesNotBlockAccept(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	cert, err := localhostCertificate()
	if err != nil {
		t.Fatal(err)
	}
	mux := NewMuxListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}})
	defer mux.(*muxListener).Listener.Close()

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	}
	go srv.Serve(mux)

	blocker, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
				DisableKeepAlives: true,
			},
			Timeout: 3 * time.Second,
		}
		resp, err := client.Get("https://" + ln.Addr().String() + "/")
		if err != nil {
			t.Errorf("second client request: %v", err)
			return
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Accept blocked while first connection idle without data")
	}
}
