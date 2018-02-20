package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

func makeTrackerURL(c client, m metainfo, host string) (*url.URL, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %s", err)
	}

	// Needs to be placed first for some stupid trackers
	q1 := u.Query()
	q1.Set("info_hash", string(c.infoHash[:]))

	q2 := u.Query()
	q2.Set("peer_id", string(c.peerID[:]))
	q2.Set("port", c.port)
	q2.Set("uploaded", "0")
	q2.Set("downloaded", "0")
	q2.Set("left", fmt.Sprintf("%d", m.totalSize))
	q2.Set("compact", "1")

	// Some stupid trackers don't accept "+" instead of "%20"
	s1 := strings.Replace(q1.Encode(), "+", "%20", 1)
	s2 := q2.Encode()
	u.RawQuery = s1 + "&" + s2

	log.Printf("tracker request URL: %s\n", u)

	return u, nil
}

func announceToTracker(c client, m metainfo, h string) ([]byte, error) {
	u, err := makeTrackerURL(c, m, h)
	if err != nil {
		return []byte{}, fmt.Errorf("tracker URL: %s", err)
	}
	resp, err := http.Get(u.String())
	if err != nil {
		return []byte{}, fmt.Errorf("HTTP GET request to tracker: %s", err)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("reading tracker response: %s", err)
	}
	return b, nil
}

func parseTrackerResponse(d1 map[string]interface{}) ([]string, error) {
	var s string
	var b1, b2 bool
	var l []interface{}
	var d2 map[string]interface{}
	var i int64

	peers := make([]string, 0)

	s, b1 = d1["failure reason"].(string)
	if b1 {
		return []string{}, fmt.Errorf("tracker returned failure response: %q", s)
	}

	s, b1 = d1["peers"].(string)
	l, b2 = d1["peers"].([]interface{})
	if !b1 && !b2 {
		return []string{}, fmt.Errorf("tracker response contains no peers entry of type string and no peers entry of type list")
	}

	if b1 {
		if len(s)%6 != 0 {
			return []string{}, fmt.Errorf("tracker response contains peers string not divisible by 6")
		}
		for i := 0; i < len(s); i += 6 {
			peer := fmt.Sprintf("%d.%d.%d.%d:%d", s[i], s[i+1], s[i+2], s[i+3], binary.BigEndian.Uint16([]byte(s[i+4:i+6])))
			peers = append(peers, peer)
		}
	} else {
		for j := 0; j < len(l); j++ {
			d2, b1 = l[j].(map[string]interface{})
			if !b1 {
				return []string{}, fmt.Errorf("tracker response contains peers list with an entry that is not a dictionary")
			}
			s, b1 = d2["ip"].(string)
			if !b1 {
				return []string{}, fmt.Errorf("tracker response contains peers list with a dictionary entry that contains no ip entry of type string")
			}
			i, b1 = d2["port"].(int64)
			if !b1 {
				return []string{}, fmt.Errorf("tracker response contains peers list with a dictionary entry that contains no port entry of type integer")
			}
			peer := fmt.Sprintf("%s:%d", s, i)
			peers = append(peers, peer)
		}
	}

	return peers, nil
}
