package core

import (
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
)

const (
	MessageMethodData = iota
	MessageMethodDial
)

type Message struct {
	Method    byte
	ConnID    uint64
	MessageID uint64
	Data      []byte
}

type MuxConn struct {
	ID    uint64
	muxWS *MuxWebSocket

	mutex sync.Mutex
	buf   []byte
	wait  chan int

	receiveMessageID uint64
	sendMessageID    *uint64
}

//client use
func NewMuxConn(muxWS *MuxWebSocket) (conn *MuxConn) {
	return &MuxConn{
		ID:            rand.Uint64(),
		muxWS:         muxWS,
		wait:          make(chan int),
		sendMessageID: new(uint64),
	}
}

func (conn *MuxConn) Write(p []byte) (n int, err error) {
	m := &Message{
		Method:    MessageMethodData,
		ConnID:    conn.ID,
		MessageID: conn.SendMessageID(),
		Data:      p,
	}

	err = conn.muxWS.SendMessage(m)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (conn *MuxConn) Read(p []byte) (n int, err error) {
	if len(conn.buf) == 0 {
		<-conn.wait
	}

	conn.mutex.Lock()
	n = copy(p, conn.buf)
	conn.buf = conn.buf[n:]
	conn.mutex.Unlock()
	return
}

func (conn *MuxConn) HandleMessage(m *Message) (err error) {
	for {
		if conn.receiveMessageID == m.MessageID {
			conn.mutex.Lock()
			conn.buf = append(conn.buf, m.Data...)
			conn.receiveMessageID++
			close(conn.wait)
			conn.wait = make(chan int)
			conn.mutex.Unlock()
			return
		}
		<-conn.wait
	}
	return
}

//client dial remote
func (conn *MuxConn) DialMessage(host string) (err error) {
	m := &Message{
		Method:    MessageMethodDial,
		MessageID: 18446744073709551615,
		ConnID:    conn.ID,
		Data:      []byte(host),
	}

	logger.Debugf("dial for %s", host)

	err = conn.muxWS.SendMessage(m)
	if err != nil {
		return
	}

	conn.muxWS.PutMuxConn(conn)
	logger.Debugf("%d %s", conn.ID, host)
	return
}

func (conn *MuxConn) SendMessageID() (id uint64) {
	id = atomic.LoadUint64(conn.sendMessageID)
	atomic.AddUint64(conn.sendMessageID, 1)
	return
}

func (conn *MuxConn) Run(c *net.TCPConn) {
	_, err := io.Copy(conn, c)
	if err != nil {
		logger.Debugf(err.Error())
	}

	go func() {
		_, err := io.Copy(c, conn)
		if err != nil {
			logger.Debugf(err.Error())
		}
	}()
	return
}