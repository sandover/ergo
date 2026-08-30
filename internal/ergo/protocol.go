package ergo

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
)

const (
	protocolVersion = 1
	maxWireBytes    = 16 * 1024 * 1024
)

type WireRequest struct {
	V       int             `json:"v"`
	Method  string          `json:"method"`
	Dir     string          `json:"dir"`
	Color   bool            `json:"color"`
	Width   int             `json:"width"`
	Stdin   string          `json:"stdin,omitempty"`
	Client  WireClient      `json:"client,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

type WireResponse struct {
	V      int    `json:"v"`
	OK     bool   `json:"ok"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Code   int    `json:"code"`
}

type WireBodyPayload struct {
	ID     string `json:"ID"`
	Append bool   `json:"Append"`
}

func readWire(conn net.Conn) (WireRequest, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return WireRequest{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || int(size) > maxWireBytes {
		return WireRequest{}, fmt.Errorf("invalid wire payload size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return WireRequest{}, err
	}
	var request WireRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return WireRequest{}, err
	}
	return request, nil
}

func writeWire(conn net.Conn, response WireResponse) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if len(payload) > maxWireBytes {
		return errors.New("wire response too large")
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err = conn.Write(payload)
	return err
}

func writeWireError(conn net.Conn, stderr string, code int) error {
	return writeWire(conn, WireResponse{V: protocolVersion, OK: false, Stderr: stderr, Code: code})
}

func writeWireOK(conn net.Conn, stdout string, code int) error {
	return writeWire(conn, WireResponse{V: protocolVersion, OK: true, Stdout: stdout, Code: code})
}

func writeWireRequest(conn net.Conn, request WireRequest) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return writeWirePayload(conn, payload)
}

func writeWirePayload(conn net.Conn, payload []byte) error {
	if len(payload) > maxWireBytes {
		return errors.New("wire payload too large")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := conn.Write(header[:]); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func readWireResponse(conn net.Conn) (WireResponse, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return WireResponse{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || int(size) > maxWireBytes {
		return WireResponse{}, fmt.Errorf("invalid wire payload size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return WireResponse{}, err
	}
	var response WireResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return WireResponse{}, err
	}
	return response, nil
}

func decodePayload[T any](payload json.RawMessage) (T, error) {
	var value T
	if len(payload) == 0 {
		return value, nil
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, err
	}
	return value, nil
}
