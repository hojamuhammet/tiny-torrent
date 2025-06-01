package client

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/anacrolix/utp"
	"github.com/hojamuhammet/tiny-torrent/bitfield"
	"github.com/hojamuhammet/tiny-torrent/peers"
	"github.com/jackpal/bencode-go"

	"github.com/hojamuhammet/tiny-torrent/message"

	"github.com/hojamuhammet/tiny-torrent/handshake"
)

type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield bitfield.Bitfield
	peer     peers.Peer
	infoHash [20]byte
	peerID   [20]byte
}

func completeHandshake(conn net.Conn, infoHash, peerID [20]byte) (*handshake.Handshake, error) {
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Clear deadline

	req := handshake.New(infoHash, peerID)
	fmt.Printf("[Handshake] Sending handshake: infoHash=%x, peerID=%x\n", req.InfoHash, req.PeerID)

	_, err := conn.Write(req.Serialize())
	if err != nil {
		fmt.Printf("[Handshake] Failed to send: %v\n", err)
		return nil, err
	}

	fmt.Println("[Handshake] Waiting for peer handshake response...")
	res, err := handshake.Read(conn)
	if err != nil {
		fmt.Printf("[Handshake] Failed to read response: %v\n", err)
		return nil, err
	}

	fmt.Printf("[Handshake] Received handshake: infoHash=%x, peerID=%x\n", res.InfoHash, res.PeerID)

	if !bytes.Equal(res.InfoHash[:], infoHash[:]) {
		fmt.Printf("[Handshake] Mismatched info hash: expected %x, got %x\n", infoHash, res.InfoHash)
		return nil, fmt.Errorf("handshake info hash mismatch")
	}

	return res, nil
}

func recvBitfield(conn net.Conn) (bitfield.Bitfield, error) {
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	defer conn.SetReadDeadline(time.Time{}) // clear deadline afterwards

	msg, err := message.Read(conn)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil, nil
		}
		return nil, err
	}

	if msg == nil { // keep-alive
		return nil, nil
	}

	switch msg.ID {
	case message.MsgBitfield:
		return msg.Payload, nil
	case message.MsgHave:
		return nil, nil
	case 20:
		return nil, nil
	default:
		return nil, nil
	}
}

func New(peer peers.Peer, peerID, infoHash [20]byte, _ int) (*Client, error) {
	var conn net.Conn
	var err error

	conn, err = utp.Dial(peer.String())
	if err != nil {
		conn, err = net.DialTimeout("tcp", peer.String(), 8*time.Second)
		if err != nil {
			return nil, fmt.Errorf("both uTP and TCP failed: %w", err)
		}
	}
	conn.SetDeadline(time.Now().Add(8 * time.Second))

	if _, err = completeHandshake(conn, infoHash, peerID); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	if _, err = conn.Write((&message.Message{ID: message.MsgInterested}).Serialize()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send interested: %w", err)
	}

	ext := map[string]interface{}{"m": map[string]int{"ut_metadata": 1}}
	var b bytes.Buffer
	if err = bencode.Marshal(&b, ext); err != nil {
		conn.Close()
		return nil, err
	}
	payload := append([]byte{0}, b.Bytes()...)
	if _, err = conn.Write((&message.Message{ID: 20, Payload: payload}).Serialize()); err != nil {
		conn.Close()
		return nil, err
	}

	bf, _ := recvBitfield(conn) // nil is fine

	return &Client{Conn: conn, Choked: true, Bitfield: bf, peer: peer,
		infoHash: infoHash, peerID: peerID}, nil
}

func (c *Client) Read() (*message.Message, error) {
	msg, err := message.Read(c.Conn)
	return msg, err
}

func (c *Client) SendRequest(index, begin, length int) error {
	req := message.FormatRequest(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	return err
}

func (c *Client) SendInterested() error {
	msg := message.Message{ID: message.MsgInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

func (c *Client) SendNotInterested() error {
	msg := message.Message{ID: message.MsgNotInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

func (c *Client) SendUnchoke() error {
	msg := message.Message{ID: message.MsgUnchoke}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

func (c *Client) SendHave(index int) error {
	msg := message.FormatHave(index)
	_, err := c.Conn.Write(msg.Serialize())
	return err
}
