package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/pieterkockx/bittorrent/bencode"
	"github.com/pieterkockx/bittorrent/pwp"
)

const (
	connWriteDeadline = 1 * time.Second
	connReadDeadline  = 2 * time.Second
)

type peerInfo struct {
	addr      string
	peerID    [20]byte
	piecesSet []bool
}

type peerConn struct {
	info   *peerInfo
	in     chan pwp.Message
	out    chan pwp.Message
	closed chan bool
}

func unpackBitmap(b []byte) []bool {
	m := make([]bool, len(b)*8)
	for j := 0; j < len(b); j++ {
		for i := 0; i < 8; i++ {
			if b[j]&(1<<(8-uint(i+1))) > 0 {
				m[8*j+i] = true
			}
		}
	}
	return m
}

func packBitmap(m []bool) []byte {
	l := len(m) / 8
	if len(m)%8 != 0 {
		l++
	}
	b := make([]byte, l)
	for j := 0; j < l; j++ {
		for i := 0; i < 8; i++ {
			if 8*j+i < len(m) && m[8*j+i] {
				b[j] |= 1 << (8 - uint(i+1))
			}
		}
	}
	return b
}

func shakeHands(c client, addr string) (peerInfo, net.Conn, error) {
	var b []byte
	var n int

	conn, err := net.DialTimeout("tcp", addr, 5000000000)
	if err != nil {
		return peerInfo{}, nil, fmt.Errorf("%s", err)
	}

	b = pwp.Handshake{InfoHash: c.infoHash, PeerID: c.peerID}.Marshal()
	conn.SetWriteDeadline(time.Now().Add(connWriteDeadline))
	n, err = conn.Write(b)
	if err != nil {
		conn.Close()
		return peerInfo{}, nil, fmt.Errorf("reading handshake (wrote %d [of %d] bytes): %s", n, len(b), err)
	}

	conn.SetReadDeadline(time.Now().Add(connReadDeadline))
	remote, err := pwp.ReadHandshake(conn)
	if err != nil {
		conn.Close()
		return peerInfo{}, nil, fmt.Errorf("reading handshake: %s", err)
	}

	b = pwp.Message{Typ: pwp.MessageBitfield, Data: packBitmap(c.piecesSet)}.Marshal()
	conn.SetWriteDeadline(time.Now().Add(connWriteDeadline))
	n, err = conn.Write(b)
	if err != nil {
		conn.Close()
		return peerInfo{}, nil, fmt.Errorf("writing %s message (wrote %d [of %d] bytes): %s", pwp.MessageBitfield, n, len(b), err)
	}

	conn.SetReadDeadline(time.Now().Add(connReadDeadline))
	m, err := pwp.ReadMessage(conn)
	if err != nil {
		conn.Close()
		return peerInfo{}, nil, fmt.Errorf("reading message: %s", err)
	}
	if m.Typ != pwp.MessageBitfield {
		conn.Close()
		return peerInfo{}, nil, fmt.Errorf("expected %s message, got %s message instead", pwp.MessageBitfield, m.Typ)
	}
	piecesSet := unpackBitmap(m.Data)[:len(c.piecesSet)]

	return peerInfo{addr: addr, peerID: remote.PeerID, piecesSet: piecesSet}, conn, nil
}

func addPeer(c client, addr string) (*peerConn, error) {
	info, conn, err := shakeHands(c, addr)
	if err != nil {
		return nil, fmt.Errorf("shaking hands: %s", err)
	}

	in := make(chan pwp.Message)
	out := make(chan pwp.Message)
	p := peerConn{&info, in, out, make(chan bool)}

	pleaseClose := make(chan bool)
	go receive(conn, pleaseClose)
	go send(conn, out, pleaseClose, p.closed)

	expect(in, pwp.Message{Typ: pwp.MessageUnchoke})

	log.Printf("peer manager: sending %s message to %s\n", pwp.MessageInterested, conn.RemoteAddr())
	out <- pwp.Message{Typ: pwp.MessageInterested}

	timeout := 5 * time.Second
	select {
	case <-in:
		log.Printf("peer manager: received %s message from %s\n", pwp.MessageUnchoke, conn.RemoteAddr())
	case <-time.After(timeout):
		conn.Close()
		<-p.closed
		return nil, fmt.Errorf("timed out waiting for %s message from %s", pwp.MessageUnchoke, conn.RemoteAddr())
	}

	return &p, nil
}

func managePeers(c client, m metainfo, hosts []string, peers chan *peerConn) {
	log.Printf("peer manager: started\n")

	i := -1
	for {
		i++
		if i == len(hosts) {
			log.Fatalln("peer manager: tried all trackers, giving up")
			i = 0
		}
		for {
			log.Printf("peer manager: trying tracker %s\n", hosts[i])
			b, err := announceToTracker(c, m, hosts[i])
			if err != nil {
				log.Printf("peer manager: announcing: %s\n", err)
				break
			}
			d, err := bencode.UnmarshalDict(b)
			if err != nil {
				log.Printf("peer manager: unmarshaling tracker response: %s\n", err)
				break
			}
			addrs, err := parseTrackerResponse(d)
			if err != nil {
				log.Printf("peer manager: parsing tracker response: %s\n", err)
				break
			}

			log.Printf("peer manager: got %d peer addresses from tracker\n", len(addrs))
			for i := 0; i < len(addrs); i++ {
				peer, err := addPeer(c, addrs[i])
				if err != nil {
					log.Printf("peer manager: adding peer: %s\n", err)
					continue
				}
				log.Printf("peer manager: succesfully connected to %s\n", addrs[i])
				peers <- peer
				log.Printf("peer manager: %s passed on, waiting for it to close\n", addrs[i])
				<-peer.closed
				log.Printf("peer manager: connection to %s was closed\n", addrs[i])
			}
			if len(addrs) == 0 {
				break
			}
		}
		log.Printf("peer manager: trying next tracker\n")
	}
}
