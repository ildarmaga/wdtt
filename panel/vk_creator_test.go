package panel

import "testing"

func TestValidateVKCookiesJSON(t *testing.T) {
	if err := validateVKCookiesJSON([]byte(`[{"name":"remixsid","value":"abc"}]`)); err != nil {
		t.Fatal(err)
	}
	if err := validateVKCookiesJSON([]byte(`[{"name":"remixlang","value":"0"}]`)); err == nil {
		t.Fatal("expected missing remixsid error")
	}
}

func TestMergeVKHashes(t *testing.T) {
	got := mergeVKHashes("hash1", "https://vk.com/call/join/hash2")
	if got != "hash1,hash2" {
		t.Fatalf("got %q", got)
	}
}

func TestSaveVKCookieString(t *testing.T) {
	dir := t.TempDir()
	old := vkSecretsDir
	vkSecretsDir = dir
	vkCookiesPath = dir + "/cookies-vk.json"
	defer func() {
		vkSecretsDir = old
		vkCookiesPath = old + "/cookies-vk.json"
	}()
	if err := saveVKCookies([]byte("remixsid=test123; remixlang=0")); err != nil {
		t.Fatal(err)
	}
	ok, _ := vkCookiesStatus()
	if !ok {
		t.Fatal("expected cookies ok")
	}
}
