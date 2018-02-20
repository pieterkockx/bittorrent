package main

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/pieterkockx/bittorrent/pwp"
)

type messageInbox struct {
	sync.Mutex
	dir map[string]chan pwp.Message
}

var inbox = messageInbox{dir: map[string]chan pwp.Message{}}

func expect(in chan pwp.Message, msg pwp.Message) {
	inbox.Lock()
	defer inbox.Unlock()
	inbox.dir[msg.Id()] = in
}

func forward(msg pwp.Message) {
	inbox.Lock()
	defer inbox.Unlock()
	ch, has := inbox.dir[msg.Id()]
	if has {
		delete(inbox.dir, msg.Id())
		ch <- msg
	} else {
		log.Printf("forward: no receiver for %s message: discarding\n", msg.Typ)
	}
}

func unforward(msg pwp.Message) {
	inbox.Lock()
	defer inbox.Unlock()
	ch, has := inbox.dir[msg.Id()]
	if has {
		delete(inbox.dir, msg.Id())
		close(ch)
	} else {
		log.Printf("warning: unforward was asked to delete non-existent entry\n")
	}
}

func receive(conn net.Conn, pleaseClose chan bool) {
	for {
		conn.SetReadDeadline(time.Now().Add(connReadDeadline))
		msg, err := pwp.ReadMessage(conn)
		if err != nil {
			log.Printf("receive: %s: please close %s\n", err, conn.RemoteAddr())
			pleaseClose <- true
			log.Printf("receive: thanks in advance for closing %s\n", conn.RemoteAddr())
			return
		}
		forward(msg)
	}
}

func send(conn net.Conn, out chan pwp.Message, pleaseClose, closed chan bool) {
	for {
		msg := pwp.Message{}

		select {
		case msg = <-out:
		case <-pleaseClose:
			log.Printf("send: was asked to close %s\n", conn.RemoteAddr())
			conn.Close()
			closed <- true
			log.Printf("send: closed %s\n", conn.RemoteAddr())
			return
		}

		conn.SetWriteDeadline(time.Now().Add(connWriteDeadline))
		_, err := conn.Write(msg.Marshal())
		if err != nil {
			log.Printf("send: %s: closing %s\n", err, conn.RemoteAddr())
			conn.Close()
			<-pleaseClose
			closed <- true
			log.Printf("send: closed %s\n", conn.RemoteAddr())
			return
		}
	}
}
