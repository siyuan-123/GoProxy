package proxy

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDialHTTPConnectRejectsNon200(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}

		fmt.Fprint(conn, "HTTP/1.1 503 Too Many Open Connections\r\nContent-Length: 22\r\n\r\nToo many open proxies")
	}()

	_, err = dialHTTPConnect(ln.Addr().String(), "example.com:443", time.Second)
	if err == nil {
		t.Fatal("expected CONNECT to fail on upstream 503")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected 503 in error, got %v", err)
	}
}

func TestDialHTTPConnectAccepts200(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}

		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	}()

	conn, err := dialHTTPConnect(ln.Addr().String(), "example.com:443", time.Second)
	if err != nil {
		t.Fatalf("expected CONNECT success, got %v", err)
	}
	conn.Close()

	<-done
}
