package client

import (
	"context"
	seekritcrypto "github.com/seekritdev/terraform-provider-seekrit/internal/crypto"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceTokenWireBoundary(t *testing.T) {
	created, err := seekritcrypto.CreateServiceToken()
	if err != nil {
		t.Fatal(err)
	}
	auth, err := seekritcrypto.ServiceTokenAuth(created.Token)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("authorization") != "Bearer "+auth {
			t.Error("wrong wire credential")
		}
		if strings.Contains(r.Header.Get("authorization"), strings.Split(created.Token, ".")[2]) {
			t.Error("private key sent")
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	if err := New(server.URL, created.Token, "test", nil).do(context.Background(), "GET", "/v1/me", nil, nil); err != nil {
		t.Fatal(err)
	}
	legacy := created.TokenID + "_" + strings.Split(created.Token, ".")[2]
	if err := New(server.URL, legacy, "test", nil).do(context.Background(), "GET", "/v1/me", nil, nil); err == nil {
		t.Error("legacy token sent")
	}
	if calls != 1 {
		t.Errorf("unexpected requests: %d", calls)
	}
}
