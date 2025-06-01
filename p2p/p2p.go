package p2p

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"log"
	"net"
	"runtime"
	"time"

	"github.com/hojamuhammet/tiny-torrent/client"
	"github.com/hojamuhammet/tiny-torrent/message"
	"github.com/hojamuhammet/tiny-torrent/peers"
	"github.com/jackpal/bencode-go"
)

const MaxBlockSize = 16384
const MaxBacklog = 5

type Torrent struct {
	Peers       []peers.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type pieceProgress struct {
	index      int
	client     *client.Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

func (state *pieceProgress) readMessage() error {
	msg, err := state.client.Read()
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		return err
	}
	if msg == nil {
		log.Printf("Keep-alive or no message received")
		return nil
	}

	switch msg.ID {
	case message.MsgUnchoke:
		state.client.Choked = false
		log.Printf("Received UNCHOKE")
	case message.MsgChoke:
		state.client.Choked = true
		log.Printf("Received CHOKE")
	case message.MsgHave:
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
		log.Printf("Received HAVE for piece %d", index)
	case message.MsgPiece:
		n, err := message.ParsePiece(state.index, state.buf, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
		log.Printf("Received PIECE data, downloaded %d bytes", n)
	case 20:
		log.Printf("Received EXTENDED message")
		if len(msg.Payload) > 0 && msg.Payload[0] == 0 {
			handleExtendedHandshake(msg.Payload[1:])
		}
	default:
		log.Printf("Received unknown message ID: %d", msg.ID)
	}
	return nil
}

func attemptDownloadPiece(c *client.Client, pw *pieceWork) ([]byte, error) {
	state := pieceProgress{
		index:  pw.index,
		client: c,
		buf:    make([]byte, pw.length),
	}

	c.Conn.SetDeadline(time.Time{})

	for state.downloaded < pw.length {
		if !state.client.Choked {
			for state.backlog < MaxBacklog && state.requested < pw.length {
				blockSize := MaxBlockSize
				if pw.length-state.requested < blockSize {
					blockSize = pw.length - state.requested
				}
				if err := c.SendRequest(pw.index, state.requested, blockSize); err != nil {
					return nil, err
				}
				state.backlog++
				state.requested += blockSize
			}
		}
		if err := state.readMessage(); err != nil {
			return nil, err
		}
	}

	return state.buf, nil
}

func checkIntegrity(pw *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("Index %d failed integrity check", pw.index)
	}
	return nil
}

func (t *Torrent) startDownloadWorker(peer peers.Peer,
	work chan *pieceWork, out chan *pieceResult) {

	c, err := client.New(peer, t.PeerID, t.InfoHash, len(t.PieceHashes))
	if err != nil {
		log.Printf("handshake %s: %v", peer, err)
		return
	}
	defer c.Conn.Close()

	gotBF := false
	c.Bitfield = make([]byte, (len(t.PieceHashes)+7)/8)

	for {
		msg, err := c.Read()
		if err != nil {
			return
		}
		if msg == nil {
			continue
		}
		switch msg.ID {
		case message.MsgUnchoke:
			c.Choked = false
			goto READY
		case message.MsgBitfield:
			c.Bitfield, gotBF = msg.Payload, true
		case message.MsgHave:
			if i, e := message.ParseHave(msg); e == nil {
				c.Bitfield.SetPiece(i)
				gotBF = true
			}
		}
	}
READY:
	for pw := range work {
		if gotBF && !c.Bitfield.HasPiece(pw.index) {
			work <- pw
			continue
		}
		buf, err := attemptDownloadPiece(c, pw)
		if err != nil {
			work <- pw
			return
		}
		if checkIntegrity(pw, buf) != nil {
			work <- pw
			continue
		}
		_ = c.SendHave(pw.index)
		out <- &pieceResult{pw.index, buf}
	}
}

func (t *Torrent) calculateBoundsForPiece(index int) (begin int, end int) {
	begin = index * t.PieceLength
	end = begin + t.PieceLength
	if end > t.Length {
		end = t.Length
	}
	return begin, end
}

func (t *Torrent) calculatePieceSize(index int) int {
	begin, end := t.calculateBoundsForPiece(index)
	return end - begin
}

func (t *Torrent) Download() ([]byte, error) {
	log.Println("Starting download for", t.Name)
	workQueue := make(chan *pieceWork, len(t.PieceHashes))
	results := make(chan *pieceResult)
	for index, hash := range t.PieceHashes {
		length := t.calculatePieceSize(index)
		workQueue <- &pieceWork{index, hash, length}
	}

	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, workQueue, results)
	}

	buf := make([]byte, t.Length)
	donePieces := 0
	for donePieces < len(t.PieceHashes) {
		res := <-results
		begin, end := t.calculateBoundsForPiece(res.index)
		copy(buf[begin:end], res.buf)
		donePieces++

		percent := float64(donePieces) / float64(len(t.PieceHashes)) * 100
		numWorkers := runtime.NumGoroutine() - 1
		log.Printf("(%0.2f%%) Downloaded piece #%d from %d peers\n", percent, res.index, numWorkers)
	}
	close(workQueue)

	return buf, nil
}

func handleExtendedHandshake(payload []byte) {
	type extendedHandshake struct {
		M            map[string]int `bencode:"m"`
		MetadataSize int            `bencode:"metadata_size"`
	}

	var hs extendedHandshake
	err := bencode.Unmarshal(bytes.NewReader(payload), &hs)
	if err != nil {
		log.Printf("Failed to parse extended handshake: %v", err)
		return
	}

	log.Printf("Extended handshake: %+v", hs)
}
