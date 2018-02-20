package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/pieterkockx/bittorrent/bencode"
)

type client struct {
	port      string
	peerID    [20]byte
	infoHash  [20]byte
	piecesSet []bool
}

type metainfo struct {
	pieceHashes [][20]byte
	pieceLength uint32
	totalSize   int64
	firstFile   *fileList
}

func (c client) String() string {
	s := fmt.Sprint("info hash: ")
	for i := 0; i < len(c.infoHash); i++ {
		s += fmt.Sprintf("%02x", c.infoHash[i])
	}
	s += fmt.Sprintf("\npeer id: %q", c.peerID[:])
	return s
}

func (m metainfo) String() string {
	return fmt.Sprintf("piece length: %d bytes\nnumber of pieces: %d\ntotal size: %d bytes\n%s\n", m.pieceLength, len(m.pieceHashes), m.totalSize, m.firstFile)
}

func parseMetainfo(v map[string]interface{}) (metainfo, error) {
	var b bool
	var i int64
	var s, prefix string
	var d map[string]interface{}
	var l1, l2 []interface{}

	m := metainfo{}

	d, b = v["info"].(map[string]interface{})
	if !b {
		return metainfo{}, fmt.Errorf("metainfo has no info entry of type dictionary")
	}

	s, b = d["pieces"].(string)
	if !b {
		return metainfo{}, fmt.Errorf("info has no pieces entry of type string")
	}
	if len(s)%20 != 0 {
		return metainfo{}, fmt.Errorf("pieces string is not a multiple of 20")
	}
	pieceHashes := make([][20]byte, len(s)/20)
	for i := 0; i < len(pieceHashes); i++ {
		copy(pieceHashes[i][:], []byte(s[i*20:i*20+20]))
	}
	m.pieceHashes = pieceHashes

	i, b = d["piece length"].(int64)
	if !b {
		return metainfo{}, fmt.Errorf("info has no piece length entry of type integer")
	}
	if i != int64(uint32(i)) {
		return metainfo{}, fmt.Errorf("piece length does not fit in uint32")
	}
	m.pieceLength = uint32(i)

	m.firstFile = &fileList{next: lastFile}

	i, single := d["length"].(int64)
	l1, multi := d["files"].([]interface{})

	if !single && !multi {
		return metainfo{}, fmt.Errorf("info has no length entry of type integer and no files entry of type list")
	}
	if single && multi {
		return metainfo{}, fmt.Errorf("info has length entry of type integer and files entry of type list")
	}

	if single {
		m.firstFile.isdir = false
		m.firstFile.size = i
		s, b = d["name"].(string)
		if !b {
			return metainfo{}, fmt.Errorf("info has no name entry of type string")
		}
		m.firstFile.path = s

		m.totalSize = i
	}
	if multi {
		m.firstFile.isdir = true
		m.firstFile.size = 0
		s, b = d["name"].(string)
		if !b {
			return metainfo{}, fmt.Errorf("info has no name entry of type string")
		}
		m.firstFile.path = "./" + s

		f := m.firstFile
		for j := 0; j < len(l1); j++ {
			d, b = l1[j].(map[string]interface{})
			if !b {
				return metainfo{}, fmt.Errorf("files entry %d is not of type dictionary", j)
			}
			i, b = d["length"].(int64)
			if !b {
				return metainfo{}, fmt.Errorf("files entry %d does not have an entry length of type integer", j)
			}
			l2, b = d["path"].([]interface{})
			if !b {
				return metainfo{}, fmt.Errorf("files entry %d does not have an entry path of type list", j)
			}

			f.next = &fileList{next: lastFile}
			f = f.next
			if len(m.firstFile.path) > 0 {
				f.path = m.firstFile.path + "/"
			} else {
				f.path = "./"
			}
			k := 0
			for ; k < len(l2)-1; k++ {
				f.isdir = true
				f.size = 0
				s, b = l2[k].(string)
				if !b {
					return metainfo{}, fmt.Errorf("path entry %d is not a string", k)
				}
				f.path += s + "/"
			}
			if len(l2) > 1 {
				prefix = f.path
				f.next = &fileList{next: lastFile}
				f = f.next
			} else {
				prefix = ""
			}
			m.totalSize += i
			f.size = i
			s, b = l2[k].(string)
			if !b {
				return metainfo{}, fmt.Errorf("path entry %d is not a string", k)
			}
			f.path += prefix + s
		}
	}

	return m, nil
}

func parseTrackerURLs(v map[string]interface{}) ([]string, error) {
	var s string
	var l1, l2 []interface{}
	var b1, b2 bool

	urls := make([]string, 0)

	s, b1 = v["announce"].(string)
	l1, b2 = v["announce-list"].([]interface{})
	if !b1 && !b2 {
		return []string{}, fmt.Errorf("metainfo has no announce entry of type string and no announce-list entry of type list")
	}
	if b1 {
		urls = append(urls, s)
	}
	if !b2 {
		return urls, nil
	}

	for i := 0; i < len(l1); i++ {
		l2, b1 = l1[i].([]interface{})
		if !b1 {
			return urls, nil
		}
		for i := 0; i < len(l2); i++ {
			s, b1 = l2[i].(string)
			if !b1 {
				return urls, nil
			}
			urls = append(urls, s)
		}
	}

	return urls, nil
}

func managePieces(clientPiecesSet []bool, out chan uint32) {
	log.Printf("piece manager: started\n")

	// Speculative pieces set
	piecesSet := make([]bool, len(clientPiecesSet))
	copy(piecesSet, clientPiecesSet)

	for {
		i := uint32(0)
		for ; int64(i) < int64(len(piecesSet)); i++ {
			if !piecesSet[i] {
				break
			}
		}
		if int64(i) == int64(len(piecesSet)) {
			n := 0
			for j := 0; j < len(clientPiecesSet); j++ {
				if clientPiecesSet[j] {
					n++
				}
			}
			if n == len(clientPiecesSet) {
				close(out)
				return
			}

			// Copy actual pieces set into speculative pieces set
			copy(piecesSet, clientPiecesSet)
			log.Printf("piece manager: copied actual pieces set into speculative pieces set (got %d [of %d] pieces)\n", n, len(clientPiecesSet))

			continue
		}
		piecesSet[i] = true

		out <- i
	}
}

func main() {
	// PART 1 - OFFLINE

	b1, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("reading torrent file (from stdin): %s\n", err)
	}

	b2, err := bencode.UnmarshalDict(b1)
	if err != nil {
		log.Fatalf("unmarshaling metainfo dictionary: %s\n", err)
	}

	infoHash, err := bencode.HashInfo(b1)
	if err != nil {
		log.Fatalf("hashing info dictionary: %s\n", err)
	}

	m, err := parseMetainfo(b2)
	if err != nil {
		log.Fatalf("parsing info dictionary: %s\n", err)
	}

	urls, err := parseTrackerURLs(b2)
	if err != nil {
		log.Fatalf("parsing tracker URLs: %s\n", err)
	}

	var peerID [20]byte
	copy(peerID[:], []byte("-PK-0100-0123456890a"))

	piecesSet, err := m.firstFile.build(m.pieceLength, m.totalSize, m.pieceHashes)
	if err != nil {
		log.Fatalf("building file tree: %s\n", err)
	}

	// metainfo is not modified from here on

	c := client{peerID: peerID, infoHash: infoHash, port: "50000", piecesSet: piecesSet}

	fmt.Printf("%s\n", m)
	fmt.Printf("%s\n", c)
	fmt.Printf("announce: %v\n", urls)
	fmt.Println("")

	// PART 2 - ONLINE

	peers := make(chan *peerConn)
	go managePeers(c, m, urls, peers)

	pieces := make(chan uint32)
	go managePieces(c.piecesSet, pieces)

	// Flow control: limit maximum outstanding pieces
	wait := make(chan int, 5)

	log.Printf("main: waiting for connection\n")
	peer := <-peers
	log.Printf("main: got connection to %s\n", peer.info.addr)

	for piece := range pieces {
		log.Printf("main: getting piece %d\n", piece)
		select {
		case wait <- 1:
		case peer = <-peers:
			log.Printf("main: got new connection to %s\n", peer.info.addr)
		}

		go func(p uint32) {
			log.Printf("main (forked): going to get piece %d\n", p)

			l := m.pieceLength
			// Last piece might have a smaller length
			if int64(p) == int64(len(c.piecesSet))-1 {
				r := uint32(m.totalSize) % m.pieceLength
				if r != 0 {
					l = r
				}
			}

			err := getPiece(peer.out, p, l, m)
			<-wait
			if err != nil {
				// Avoid sending on closed pieces channel.
				// Note that the pieces channel can only be closed if the whole piecesSet is true
				if !c.piecesSet[p] {
					log.Printf("main (forked): geting piece %d: %s: putting it back in queue\n", p, err)
					pieces <- p
				} else {
					log.Printf("main (forked): geting piece %d: %s, but already got it\n", p, err)
				}
				return
			}
			log.Printf("main (forked): got piece %d\n", p)
			c.piecesSet[p] = true
		}(piece)
	}
	log.Printf("main: finished succesfully\n")
}
