package pwp

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageType byte

const (
	MessageChoke MessageType = iota
	MessageUnchoke
	MessageInterested
	MessageNotInterested
	MessageHave
	MessageBitfield
	MessageRequest
	MessagePiece
	MessageCancel
)

var messageTypeToString = map[MessageType]string{
	MessageChoke:         "choke",
	MessageUnchoke:       "unchoke",
	MessageInterested:    "interested",
	MessageNotInterested: "not interested",
	MessageHave:          "have",
	MessageBitfield:      "bitfield",
	MessageRequest:       "request",
	MessagePiece:         "piece",
	MessageCancel:        "cancel",
}

func (t MessageType) String() string {
	if s, has := messageTypeToString[t]; has {
		return s
	}
	return fmt.Sprintf("unknown (%d)", int(t))
}

type Handshake struct {
	InfoHash [20]byte
	PeerID   [20]byte
}

type Message struct {
	Typ         MessageType
	PieceIndex  uint32
	BlockOffset uint32
	BlockLength uint32
	Data        []byte
}

func (h Handshake) Marshal() []byte {
	b := make([]byte, 68)
	b[0] = 19
	copy(b[1:20], []byte("BitTorrent protocol"))
	// Leave next 8 bytes set to zero
	copy(b[28:48], h.InfoHash[:])
	copy(b[48:68], h.PeerID[:])
	return b
}

func (msg Message) Id() string {
	m := Message{}
	m.Typ = msg.Typ
	m.PieceIndex = msg.PieceIndex
	m.BlockOffset = msg.BlockOffset
	m.BlockLength = msg.BlockLength
	return string(m.Marshal())
}

func (msg Message) Marshal() []byte {
	b := make([]byte, 5)
	length := 1
	switch msg.Typ {
	case MessageChoke:
		fallthrough
	case MessageUnchoke:
		fallthrough
	case MessageInterested:
		fallthrough
	case MessageNotInterested:
		/* break */
	case MessageHave:
		b = append(b, make([]byte, 4)...)
		binary.BigEndian.PutUint32(b[5:9], msg.PieceIndex)
		length += 4
	case MessageRequest:
		fallthrough
	case MessageCancel:
		b = append(b, make([]byte, 12)...)
		binary.BigEndian.PutUint32(b[5:9], msg.PieceIndex)
		binary.BigEndian.PutUint32(b[9:13], msg.BlockOffset)
		binary.BigEndian.PutUint32(b[13:17], msg.BlockLength)
		length += 12
	case MessageBitfield:
		b = append(b, msg.Data...)
		length += len(msg.Data)
	case MessagePiece:
		b = append(b, make([]byte, 8)...)
		binary.BigEndian.PutUint32(b[5:9], msg.PieceIndex)
		binary.BigEndian.PutUint32(b[9:13], msg.BlockOffset)
		length += 8
		b = append(b, msg.Data...)
		length += len(msg.Data)
	default:
		panic(fmt.Sprintf("marshaling message: message has unknown type (%d)", int(msg.Typ)))
	}
	b[4] = byte(msg.Typ)
	if int64(length) != int64(uint32(length)) {
		panic(fmt.Sprintf("marshaling message: length=%d overflows integer", length))
	}
	binary.BigEndian.PutUint32(b[:4], uint32(length))
	return b
}

func unmarshalHandshake(b []byte) (Handshake, error) {
	if b[0] != 19 || string(b[1:20]) != "BitTorrent protocol" {
		return Handshake{}, fmt.Errorf("unknown protocol")
	}
	// Ignore next 8 bytes
	h := Handshake{}
	copy(h.PeerID[:], b[48:68])
	return h, nil
}

func unmarshalMessage(b []byte) (Message, error) {
	if len(b) == 0 {
		return Message{}, fmt.Errorf("message has length 0")
	}
	typ := MessageType(b[0])
	msg := Message{Typ: typ}
	switch typ {
	case MessageChoke:
		fallthrough
	case MessageUnchoke:
		fallthrough
	case MessageInterested:
		fallthrough
	case MessageNotInterested:
		break
	case MessageHave:
		msg.PieceIndex = binary.BigEndian.Uint32(b[1:5])
	case MessageRequest:
		fallthrough
	case MessageCancel:
		msg.PieceIndex = binary.BigEndian.Uint32(b[1:5])
		msg.BlockOffset = binary.BigEndian.Uint32(b[5:9])
		msg.BlockLength = binary.BigEndian.Uint32(b[9:13])
	case MessageBitfield:
		msg.Data = b[1:]
	case MessagePiece:
		msg.PieceIndex = binary.BigEndian.Uint32(b[1:5])
		msg.BlockOffset = binary.BigEndian.Uint32(b[5:9])
		msg.Data = b[9:]
	default:
		return Message{}, fmt.Errorf("message has unknown type (%d)", int(typ))
	}
	return msg, nil
}

func ReadMessage(rr io.Reader) (Message, error) {
	var l [4]byte
	n, err := io.ReadFull(rr, l[:])
	if err != nil {
		return Message{}, fmt.Errorf("reading message length (read %d [of %d] bytes): %s", n, len(l), err)
	}
	u := binary.BigEndian.Uint32(l[:])
	buf := make([]byte, u)
	n, err = io.ReadFull(rr, buf)
	if err != nil {
		return Message{}, fmt.Errorf("reading message body (read %d [of %d] bytes): %s", n, len(buf), err)
	}
	return unmarshalMessage(buf)
}

func ReadHandshake(rr io.Reader) (Handshake, error) {
	buf := make([]byte, 68)
	n, err := io.ReadFull(rr, buf)
	if err != nil {
		return Handshake{}, fmt.Errorf("reading handshake (read %d [of %d] bytes): %s", n, len(buf), err)
	}
	return unmarshalHandshake(buf)
}
