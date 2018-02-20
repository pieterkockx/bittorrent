package main

import (
	"crypto/sha1"
	"fmt"
	"time"

	"github.com/pieterkockx/bittorrent/pwp"
)

const (
	blockLength  = uint32(0x4000)
	pieceTimeout = 10 * time.Second
)

type piece struct {
	index uint32
	data  []byte
}

func fetchPiece(out chan pwp.Message, index, pieceLength uint32, hash [20]byte) (piece, error) {
	data := make([]byte, 0)
	for offs := uint32(0); offs < pieceLength; offs += blockLength {
		l := blockLength
		if offs+blockLength > pieceLength {
			l = pieceLength - offs
		}

		in := make(chan pwp.Message)
		msg := pwp.Message{Typ: pwp.MessagePiece, PieceIndex: index, BlockOffset: offs}
		expect(in, msg)

		out <- pwp.Message{pwp.MessageRequest, index, offs, l, []byte{}}

		inmsg := pwp.Message{}
		select {
		case inmsg = <-in:
		case <-time.After(pieceTimeout):
			unforward(msg)
			return piece{}, fmt.Errorf("timed out after %s", pieceTimeout)
		}

		data = append(data, inmsg.Data...)
	}
	if sha1.Sum(data) != hash {
		return piece{}, fmt.Errorf("hash differs")
	}
	return piece{index, data}, nil
}

func storePiece(p piece, pieceLength uint32, f *fileList) error {
	totoffs := int64(pieceLength) * int64(p.index)
	begin := int64(0)
	end := f.size
	for len(p.data) > 0 {
		for totoffs >= end {
			// f.next is not lastFile
			f = f.next
			begin = end
			end += f.size
		}
		max := int64(len(p.data))
		if max > end-totoffs {
			max = end - totoffs
		}
		_, err := f.file.WriteAt(p.data[:max], totoffs-begin)
		if err != nil {
			return err
		}
		// nwritten != max
		totoffs += max
		p.data = p.data[max:]
	}
	return nil
}

func getPiece(from chan pwp.Message, index uint32, length uint32, m metainfo) error {
	p, err := fetchPiece(from, index, length, m.pieceHashes[index])
	if err != nil {
		return fmt.Errorf("fetching piece: %s", err)
	}
	err = storePiece(p, m.pieceLength, m.firstFile)
	if err != nil {
		return fmt.Errorf("storing piece: %s", err)
	}
	return nil
}
