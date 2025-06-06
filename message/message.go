package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

type messageID uint8

const (
	MsgChoke         messageID = 0
	MsgUnchoke       messageID = 1
	MsgInterested    messageID = 2
	MsgNotInterested messageID = 3
	MsgHave          messageID = 4
	MsgBitfield      messageID = 5
	MsgRequest       messageID = 6
	MsgPiece         messageID = 7
	MsgCancel        messageID = 8
	MsgPort          messageID = 9
	MsgExtended      messageID = 20
)

type Message struct {
	ID      messageID
	Payload []byte
}

func FormatRequest(index, begin, length int) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	msg := &Message{ID: MsgRequest, Payload: payload}
	fmt.Printf("[Serialize] Created Request message - Index: %d, Begin: %d, Length: %d\n", index, begin, length)
	return msg
}

func FormatHave(index int) *Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(index))
	return &Message{ID: MsgHave, Payload: payload}
}

func ParsePiece(index int, buf []byte, msg *Message) (int, error) {
	if msg.ID != MsgPiece {
		return 0, fmt.Errorf("Expected PIECE (ID %d), got ID %d", MsgPiece, msg.ID)
	}
	if len(msg.Payload) < 8 {
		return 0, fmt.Errorf("Payload too short. %d < 8", len(msg.Payload))
	}
	parsedIndex := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
	if parsedIndex != index {
		return 0, fmt.Errorf("Expected index %d, got %d", index, parsedIndex)
	}
	begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
	if begin >= len(buf) {
		return 0, fmt.Errorf("Begin offset too high. %d >= %d", begin, len(buf))
	}
	data := msg.Payload[8:]
	if begin+len(data) > len(buf) {
		return 0, fmt.Errorf("Data too long [%d] for offset %d with length %d", len(data), begin, len(buf))
	}
	copy(buf[begin:], data)
	return len(data), nil
}

func ParseHave(msg *Message) (int, error) {
	if msg.ID != MsgHave {
		return 0, fmt.Errorf("Expected HAVE (ID %d), got ID %d", MsgHave, msg.ID)
	}
	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("Expected payload length 4, got length %d", len(msg.Payload))
	}
	index := int(binary.BigEndian.Uint32(msg.Payload))
	return index, nil
}

func (m *Message) Serialize() []byte {
	if m == nil {
		fmt.Println("[Serialize] KeepAlive message")
		return make([]byte, 4)
	}
	length := uint32(len(m.Payload) + 1)
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = byte(m.ID)
	copy(buf[5:], m.Payload)
	fmt.Printf("[Serialize] Serializing message - ID: %s (%d), Payload Length: %d, Payload: %x\n", m.name(), m.ID, len(m.Payload), m.Payload)
	return buf
}

func Read(r io.Reader) (*Message, error) {
	lengthBuf := make([]byte, 4)
	fmt.Println("[Read] Waiting to read message length...")
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		fmt.Printf("[Read] Error reading length: %v\n", err)
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)
	fmt.Printf("[Read] Message length: %d\n", length)

	if length == 0 {
		fmt.Println("[Read] KeepAlive message received")
		return nil, nil
	}

	messageBuf := make([]byte, length)
	fmt.Printf("[Read] Reading message payload of %d bytes...\n", length)
	_, err = io.ReadFull(r, messageBuf)
	if err != nil {
		fmt.Printf("[Read] Error reading payload: %v\n", err)
		return nil, err
	}

	m := Message{
		ID:      messageID(messageBuf[0]),
		Payload: messageBuf[1:],
	}

	fmt.Printf("[Read] Received message: ID=%s (%d), Payload Length: %d, Payload=%x\n", m.name(), m.ID, len(m.Payload), m.Payload)

	if m.ID == MsgUnchoke {
		fmt.Println("[Protocol] Received Unchoke - Ready to request pieces")
	}

	return &m, nil
}

func (m *Message) name() string {
	if m == nil {
		return "KeepAlive"
	}
	switch m.ID {
	case MsgChoke:
		return "Choke"
	case MsgUnchoke:
		return "Unchoke"
	case MsgInterested:
		return "Interested"
	case MsgNotInterested:
		return "NotInterested"
	case MsgHave:
		return "Have"
	case MsgBitfield:
		return "Bitfield"
	case MsgRequest:
		return "Request"
	case MsgPiece:
		return "Piece"
	case MsgCancel:
		return "Cancel"
	case MsgPort:
		return "Port"
	case MsgExtended:
		return "Extended"
	default:
		return fmt.Sprintf("Unknown#%d", m.ID)
	}
}

func (m *Message) String() string {
	if m == nil {
		return m.name()
	}
	return fmt.Sprintf("%s [%d]", m.name(), len(m.Payload))
}
