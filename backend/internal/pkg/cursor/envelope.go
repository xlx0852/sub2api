package cursor

import (
	"encoding/binary"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

const (
	ConnectFlagData      byte = 0x00
	ConnectFlagEndStream byte = 0x02
)

// EncodeEnvelope creates a 5-byte Connect envelope framing for a protobuf message.
func EncodeEnvelope(msg proto.Message) ([]byte, error) {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal proto message: %w", err)
	}
	buf := make([]byte, 5+len(data))
	buf[0] = ConnectFlagData
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(data)))
	copy(buf[5:], data)
	return buf, nil
}

// ReadEnvelope reads a single 5-byte Connect frame from an io.Reader.
// Returns flag (0x00 for data, 0x02 for trailer/end-of-stream), payload bytes, and error.
func ReadEnvelope(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	flag := header[0]
	length := binary.BigEndian.Uint32(header[1:5])
	if length == 0 {
		return flag, nil, nil
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return flag, nil, fmt.Errorf("read frame payload (len=%d): %w", length, err)
	}
	return flag, payload, nil
}
