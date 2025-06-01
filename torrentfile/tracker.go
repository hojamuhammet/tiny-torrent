package torrentfile

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hojamuhammet/tiny-torrent/peers"

	"github.com/jackpal/bencode-go"
)

type bencodeTrackerResp struct {
	Interval int    `bencode:"interval"`
	Peers    string `bencode:"peers"`
}

func buildTrackerURL(baseURL string, infoHash, peerID [20]byte, port uint16, length int) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"1"},
		"left":       []string{strconv.Itoa(length)},
	}

	params.Set("info_hash", "PLACEHOLDER1")
	params.Set("peer_id", "PLACEHOLDER2")
	base.RawQuery = params.Encode()

	base.RawQuery = fixRawQueryEncoding(base.RawQuery, "info_hash", infoHash[:])
	base.RawQuery = fixRawQueryEncoding(base.RawQuery, "peer_id", peerID[:])

	return base.String(), nil
}

func (t *TorrentFile) requestPeers(peerID [20]byte, port uint16) ([]peers.Peer, error) {
	announceURL, err := buildTrackerURL(t.Announce, t.InfoHash, peerID, port, t.Length)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(announceURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Validate status code and content type
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tracker returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		fmt.Printf("[Warning] Unexpected Content-Type: %s\n", ct)
	}

	// Debug print the raw tracker response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	fmt.Printf("[Tracker Response] Raw body:\n%s\n", string(body))

	// Attempt to decode the bencoded tracker response
	var tr bencodeTrackerResp
	if err := bencode.Unmarshal(bytes.NewReader(body), &tr); err != nil {
		return nil, fmt.Errorf("bencode unmarshal error: %w", err)
	}

	rawPeers, err := peers.Unmarshal([]byte(tr.Peers))
	if err != nil {
		return nil, err
	}

	return dedupPeers(rawPeers), nil
}

func fixRawQueryEncoding(query, key string, val []byte) string {
	encoded := ""
	for _, b := range val {
		encoded += fmt.Sprintf("%%%02X", b)
	}
	return strings.Replace(query, key+"=PLACEHOLDER"+map[string]string{
		"info_hash": "1",
		"peer_id":   "2",
	}[key], key+"="+encoded, 1)
}

func dedupPeers(list []peers.Peer) []peers.Peer {
	seen := make(map[string]struct{}, len(list))
	unique := make([]peers.Peer, 0, len(list))
	for _, p := range list {
		ip := p.IP.String()
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		unique = append(unique, p)
	}
	return unique
}
