package messaging

import (
	"fmt"
	"io"
	"log"
	"net"
	"testing"
)

// ---------------------------------------------------------------------------
// Fake AMQP 0-9-1 server for unit tests (no broker required)
// ---------------------------------------------------------------------------

// silenceLoggers suppresses log output during tests and restores the
// original loggers when the test finishes.
func silenceLoggers(t *testing.T) {
	t.Helper()
	origInfo, origWarn, origError := Info, Warn, Error
	quiet := log.New(io.Discard, "", 0)
	Info, Warn, Error = quiet, quiet, quiet
	t.Cleanup(func() {
		Info, Warn, Error = origInfo, origWarn, origError
	})
}

// startFakeAMQPServer starts a minimal TCP listener that speaks just enough
// of AMQP 0-9-1 to let amqp091-go complete its handshake. It returns the
// AMQP URI and a cleanup function.
//
// The protocol flow is:
//  1. Client sends "AMQP\x00\x00\x09\x01" (protocol header).
//  2. Server sends Connection.Start (method 10,10).
//  3. Client sends Connection.StartOk.
//  4. Server sends Connection.Tune.
//  5. Client sends Connection.TuneOk.
//  6. Client sends Connection.Open.
//  7. Server sends Connection.OpenOk.
//
// After that the connection is "open" from the client library's perspective.
func startFakeAMQPServer(t *testing.T) (uri string, cleanup func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go handleFakeAMQPConn(conn)
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	uri = fmt.Sprintf("amqp://guest:guest@127.0.0.1:%d/", addr.Port)
	cleanup = func() { _ = ln.Close() }
	return uri, cleanup
}

// handleFakeAMQPConn drives the server side of a minimal AMQP handshake
// and then keeps the connection open until the client disconnects.
func handleFakeAMQPConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// 1. Read protocol header (8 bytes): "AMQP\x00\x00\x09\x01"
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}

	// 2. Send Connection.Start
	writeFrame(conn, 1, 0, connectionStart())

	// 3. Read Connection.StartOk frame
	if !readFrame(conn) {
		return
	}

	// 4. Send Connection.Tune
	writeFrame(conn, 1, 0, connectionTune())

	// 5. Read Connection.TuneOk frame
	if !readFrame(conn) {
		return
	}

	// 6. Read Connection.Open frame
	if !readFrame(conn) {
		return
	}

	// 7. Send Connection.OpenOk
	writeFrame(conn, 1, 0, connectionOpenOk())

	// Keep connection open, reading frames until the client disconnects.
	for {
		frameType, channel, payload, ok := readFrameFull(conn)
		if !ok {
			return
		}
		// Method frame
		if frameType == 1 && len(payload) >= 4 {
			classID := uint16(payload[0])<<8 | uint16(payload[1])
			methodID := uint16(payload[2])<<8 | uint16(payload[3])

			switch {
			case classID == 20 && methodID == 10:
				// Channel.Open -> Channel.OpenOk
				writeFrame(conn, 1, channel, channelOpenOk())
			case classID == 10 && methodID == 50:
				// Connection.Close -> Connection.CloseOk
				writeFrame(conn, 1, 0, connectionCloseOk())
				return
			case classID == 20 && methodID == 40:
				// Channel.Close -> Channel.CloseOk
				writeFrame(conn, 1, channel, channelCloseOk())
			case classID == 40 && methodID == 10:
				// Exchange.Declare -> Exchange.DeclareOk
				writeFrame(conn, 1, channel, exchangeDeclareOk())
			case classID == 50 && methodID == 10:
				// Queue.Declare -> Queue.DeclareOk
				writeFrame(conn, 1, channel, queueDeclareOk())
			case classID == 50 && methodID == 20:
				// Queue.Bind -> Queue.BindOk
				writeFrame(conn, 1, channel, queueBindOk())
			case classID == 60 && methodID == 20:
				// Basic.Consume -> Basic.ConsumeOk
				writeFrame(conn, 1, channel, basicConsumeOk())
			case classID == 60 && methodID == 30:
				// Basic.Cancel -> Basic.CancelOk
				writeFrame(conn, 1, channel, basicCancelOk())
			case classID == 60 && methodID == 10:
				// Basic.Qos -> Basic.QosOk
				writeFrame(conn, 1, channel, basicQosOk())
			case classID == 85 && methodID == 10:
				// Confirm.Select -> Confirm.SelectOk
				writeFrame(conn, 1, channel, confirmSelectOk())
			default:
				// Ignore other frames
			}
		}
	}
}

// --- AMQP frame encoding/decoding helpers ---

func writeFrame(conn net.Conn, frameType byte, channel uint16, payload []byte) {
	size := uint32(len(payload))
	frame := make([]byte, 7+len(payload)+1)
	frame[0] = frameType
	frame[1] = byte(channel >> 8)
	frame[2] = byte(channel)
	frame[3] = byte(size >> 24)
	frame[4] = byte(size >> 16)
	frame[5] = byte(size >> 8)
	frame[6] = byte(size)
	copy(frame[7:], payload)
	frame[len(frame)-1] = 0xCE // frame-end
	_, _ = conn.Write(frame)
}

func readFrame(conn net.Conn) bool {
	_, _, _, ok := readFrameFull(conn)
	return ok
}

func readFrameFull(conn net.Conn) (frameType byte, channel uint16, payload []byte, ok bool) {
	hdr := make([]byte, 7)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return 0, 0, nil, false
	}
	frameType = hdr[0]
	channel = uint16(hdr[1])<<8 | uint16(hdr[2])
	size := uint32(hdr[3])<<24 | uint32(hdr[4])<<16 | uint32(hdr[5])<<8 | uint32(hdr[6])
	payload = make([]byte, size+1) // +1 for frame-end
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, 0, nil, false
	}
	payload = payload[:size] // trim frame-end marker
	return frameType, channel, payload, true
}

// --- AMQP method payload builders ---

func connectionStart() []byte {
	payload := []byte{
		0, 10, 0, 10, // class-id=10, method-id=10
		0,          // version-major
		9,          // version-minor
		0, 0, 0, 0, // server-properties: empty table (length=0)
	}
	mech := []byte("PLAIN")
	payload = appendLongstr(payload, mech)
	locale := []byte("en_US")
	payload = appendLongstr(payload, locale)
	return payload
}

func connectionTune() []byte {
	return []byte{
		0, 10, 0, 30, // class=10, method=30
		0, 0, // channel-max = 0 (no limit)
		0, 0, 0, 0, // frame-max = 0 (no limit)
		0, 0, // heartbeat = 0
	}
}

func connectionOpenOk() []byte {
	return []byte{
		0, 10, 0, 41, // class=10, method=41
		0, // reserved (shortstr, length 0)
	}
}

func connectionCloseOk() []byte {
	return []byte{0, 10, 0, 51}
}

func channelOpenOk() []byte {
	return []byte{
		0, 20, 0, 11, // class=20, method=11
		0, 0, 0, 0, // reserved (longstr, length=0)
	}
}

func channelCloseOk() []byte {
	return []byte{0, 20, 0, 41}
}

func exchangeDeclareOk() []byte {
	return []byte{0, 40, 0, 11}
}

func queueDeclareOk() []byte {
	return []byte{
		0, 50, 0, 11, // class=50, method=11
		0, 0, // queue name (shortstr, length=0 — server-named)
		0, 0, 0, 0, // message-count
		0, 0, 0, 0, // consumer-count
	}
}

func queueBindOk() []byte {
	return []byte{0, 50, 0, 21}
}

func basicConsumeOk() []byte {
	tag := []byte("ctag")
	payload := []byte{0, 60, 0, 21, byte(len(tag))}
	payload = append(payload, tag...)
	return payload
}

func basicCancelOk() []byte {
	tag := []byte("ctag")
	payload := []byte{0, 60, 0, 31, byte(len(tag))}
	payload = append(payload, tag...)
	return payload
}

func basicQosOk() []byte {
	return []byte{0, 60, 0, 11}
}

func confirmSelectOk() []byte {
	return []byte{0, 85, 0, 11}
}

func appendLongstr(buf, s []byte) []byte {
	l := uint32(len(s))
	buf = append(buf, byte(l>>24), byte(l>>16), byte(l>>8), byte(l))
	buf = append(buf, s...)
	return buf
}
