package ergo

import (
	"encoding/json"
	"net"
	"testing"
)

func TestWireBodyPayloadRoundtrip(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		request, err := readWire(server)
		if err != nil {
			t.Errorf("readWire: %v", err)
			return
		}
		payload, err := decodePayload[WireBodyPayload](request.Payload)
		if err != nil {
			t.Errorf("decode payload: %v", err)
			return
		}
		if payload.ID != "ABC" || !payload.Append || request.Stdin != "{\nline\n" {
			t.Errorf("payload = %+v stdin=%q", payload, request.Stdin)
		}
		_ = writeWireOK(server, "ABC body: 7 bytes\n", 0)
	}()

	raw, _ := json.Marshal(WireRequest{
		V: protocolVersion, Method: "body", Dir: "/tmp/project",
		Stdin: "{\nline\n",
		Payload: []byte(`{"ID":"ABC","Append":true}`),
	})
	if err := writeWirePayload(client, raw); err != nil {
		t.Fatal(err)
	}
	resp, err := readWireResponse(client)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Stdout == "" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestWireRejectsDirMismatch(t *testing.T) {
	dir := t.TempDir()
	if _, err := InitializeRepository(dir); err != nil {
		t.Fatal(err)
	}
	app := NewApplication(RepositoryOptions{StartDir: dir})
	session, err := app.OpenSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	done := make(chan WireResponse, 1)
	go func() {
		ServeSession(session, server)
	}()
	go func() {
		raw, _ := json.Marshal(WireRequest{
			V: protocolVersion, Method: "list", Dir: "/wrong/root", Payload: []byte(`{}`),
		})
		_ = writeWirePayload(client, raw)
		resp, err := readWireResponse(client)
		if err != nil {
			t.Errorf("read response: %v", err)
			return
		}
		done <- resp
	}()

	resp := <-done
	if resp.OK {
		t.Fatal("expected dir mismatch failure")
	}
	if resp.Stderr == "" {
		t.Fatal("expected stderr message")
	}
}
