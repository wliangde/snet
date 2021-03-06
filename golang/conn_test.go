package snet

import (
	"bytes"
	"encoding/hex"
	"github.com/funny/utest"
	"io"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"
)

type unstableListener struct {
	net.Listener
}

func (l *unstableListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &unstableConn{Conn: conn}, nil
}

type unstableConn struct {
	net.Conn
	enable bool
}

func (c *unstableConn) Write(b []byte) (int, error) {
	if c.enable {
		if rand.Intn(10000) < 500 {
			c.Conn.Close()
		}
	}
	return c.Conn.Write(b)
}

func (c *unstableConn) Read(b []byte) (int, error) {
	if c.enable {
		if rand.Intn(10000) < 100 {
			c.Conn.Close()
		}
	}
	return c.Conn.Read(b)
}

func RandBytes(n int) []byte {
	n = rand.Intn(n) + 1
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(rand.Intn(255))
	}
	return b
}

func ConnTest(t *testing.T, unstable, encrypt bool) {
	config := Config{
		EnableCrypt:        encrypt,
		HandshakeTimeout:   time.Second * 5,
		RewriterBufferSize: 1024,
		ReconnWaitTimeout:  time.Minute * 5,
	}

	listener, err := Listen(config, func() (net.Listener, error) {
		l, err := net.Listen("tcp", "0.0.0.0:0")
		if err != nil {
			return nil, err
		}
		return &unstableListener{l}, nil
	})
	if err != nil {
		t.Fatalf("listen failed: %s", err.Error())
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Fatalf("accept failed: %s", err.Error())
			return
		}
		if unstable {
			conn.(*Conn).base.(*unstableConn).enable = true
		}
		io.Copy(conn, conn)
		conn.Close()
		t.Log("copy exit")
		wg.Done()
	}()

	conn, err := Dial(config, func() (net.Conn, error) {
		return net.Dial("tcp", listener.Addr().String())
	})
	if err != nil {
		t.Fatalf("dial stable conn failed: %s", err.Error())
		return
	}
	defer conn.Close()

	t.Log(conn.LocalAddr())
	t.Log(conn.RemoteAddr())

	err = conn.SetDeadline(time.Time{})
	utest.IsNilNow(t, err)

	err = conn.SetReadDeadline(time.Time{})
	utest.IsNilNow(t, err)

	err = conn.SetWriteDeadline(time.Time{})
	utest.IsNilNow(t, err)

	uconn := &unstableConn{nil, unstable}
	for i := 0; i < 100000; i++ {
		if unstable && conn.(*Conn).base != uconn {
			uconn.Conn = conn.(*Conn).base
			conn.(*Conn).base = uconn
		}

		b := RandBytes(100)
		c := b
		if encrypt {
			c = make([]byte, len(b))
			copy(c, b)
		}

		if _, err := conn.Write(b); err != nil {
			t.Fatalf("write failed: %s", err.Error())
			return
		}

		a := make([]byte, len(b))
		if _, err := io.ReadFull(conn, a); err != nil {
			t.Fatalf("read failed: %s", err.Error())
			return
		}

		if !bytes.Equal(a, c) {
			println("i =", i)
			println("a =", hex.EncodeToString(a))
			println("c =", hex.EncodeToString(c))
			t.Fatalf("a != c")
			return
		}
	}

	conn.Close()
	listener.Close()

	wg.Wait()
}

func Test_Stable_NoEncrypt(t *testing.T) {
	ConnTest(t, false, false)
}

func Test_Unstable_NoEncrypt(t *testing.T) {
	ConnTest(t, true, false)
}

func Test_Stable_Encrypt(t *testing.T) {
	ConnTest(t, false, true)
}

func Test_Unstable_Encrypt(t *testing.T) {
	ConnTest(t, true, true)
}
