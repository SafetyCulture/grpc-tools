package fixture

import (
	"bytes"
	"encoding/json"
	"github.com/bradleyjkemp/grpc-tools/internal"
	"github.com/bradleyjkemp/grpc-tools/internal/proto_decoder"
	"io"
	"os"
)

type fixtureStruct struct {
	fixture map[string]*messageTree
	encoder proto_decoder.MessageEncoder
	decoder proto_decoder.MessageDecoder
}

type messageTree struct {
	message *internal.Message
	called          bool
	parent          *messageTree
	nextMessages    []*messageTree
}

// load fixture creates a Trie-like structure of messages
func loadFixture(dumpPath string, encoder proto_decoder.MessageEncoder, decoder proto_decoder.MessageDecoder) (*fixtureStruct, error) {
	dumpFile, err := os.Open(dumpPath)
	if err != nil {
		return nil, err
	}

	dumpDecoder := json.NewDecoder(dumpFile)
	fixtureStruct := fixtureStruct{
		fixture: map[string]*messageTree{},
		encoder: encoder,
		decoder: decoder,
	}

	for {
		rpc := internal.RPC{}
		err := dumpDecoder.Decode(&rpc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if fixtureStruct.fixture[rpc.StreamName()] == nil {
			fixtureStruct.fixture[rpc.StreamName()] = &messageTree{}
		}
		messageTreeNode := fixtureStruct.fixture[rpc.StreamName()]
		for _, msg := range rpc.Messages {
			msgBytes, err := encoder.Encode(rpc.StreamName(), msg)
			if err != nil {
				return nil, err
			}
			var foundExisting *messageTree
			for _, nextMessage := range messageTreeNode.nextMessages {
				if nextMessage.message.MessageOrigin == msg.MessageOrigin && bytes.Compare(msgBytes, nextMessage.message.RawMessage) == 0 {
					foundExisting = nextMessage
					break
				}
			}
			if foundExisting == nil {
				foundExisting = &messageTree{
					message: msg,
					nextMessages: nil,
				}
				messageTreeNode.nextMessages = append(messageTreeNode.nextMessages, foundExisting)
			}

			messageTreeNode = foundExisting
		}
	}

	return &fixtureStruct, nil
}
