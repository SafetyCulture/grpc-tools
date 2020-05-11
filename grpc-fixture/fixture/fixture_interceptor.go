package fixture

import (
	"github.com/bradleyjkemp/grpc-tools/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"time"
)

// fixtureInterceptor implements a gRPC.StreamingServerInterceptor that replays saved responses
func (f fixtureStruct) intercept(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, _ grpc.StreamHandler) error {
	messageTreeNode := f.fixture[info.FullMethod]
	var unprocessedReceivedMessage []byte
	if messageTreeNode == nil {
		return status.Error(codes.Unavailable, "no saved responses found for method "+info.FullMethod)
	}

	for {
		// possibility that server sends the first method
		serverFirst := len(messageTreeNode.nextMessages) > 0
		for _, message := range messageTreeNode.nextMessages {
			serverFirst = serverFirst && message.message.MessageOrigin == internal.ServerMessage
		}

		if serverFirst {
			var hasUncalled bool
			for _, message := range messageTreeNode.nextMessages {
				if message.message.MessageOrigin == internal.ServerMessage && message.called != true {
					if message.called == false {
						hasUncalled = true
					}
					if info.FullMethod == "/s12.tasks.v1.ActionsService/GetAction" {
						msgBytes, encodeErr := f.encoder.Encode(info.FullMethod, message.message)
						if encodeErr != nil {
							return encodeErr
						}
						sendMsgErr := ss.SendMsg(msgBytes)
						if sendMsgErr != nil {
							return sendMsgErr
						}
					} else {
						msgBytes := message.message.RawMessage
						sendMsgErr := ss.SendMsg(msgBytes)
						if sendMsgErr != nil {
							return sendMsgErr
						}
					}
					// recurse deeper into the tree
					message.called = true
					unprocessedReceivedMessage = nil
					message.parent = messageTreeNode
					messageTreeNode = message
					// found the first server message so break
					break
				}
			}
			if hasUncalled == false {
				messageTreeNode.called = true
				messageTreeNode = messageTreeNode.parent
			}
		} else {
			// wait for a client message and then proceed based on its contents
			var receivedMessage []byte
			if unprocessedReceivedMessage == nil {
				err := ss.RecvMsg(&receivedMessage)
				unprocessedReceivedMessage = receivedMessage
				receivedMessageStructure := internal.Message{
					MessageOrigin: internal.ClientMessage,
					RawMessage:    receivedMessage,
					Message:       nil,
					Timestamp:     time.Time{},
				}
				if info.FullMethod == "/s12.tasks.v1.ActionsService/GetAction" {
					receivedMessageDecoded, decodeErr := f.decoder.Decode(info.FullMethod, &receivedMessageStructure)
					if receivedMessageDecoded == nil {
						return nil
					}
					if decodeErr != nil {
						return decodeErr
					}
					test := messageTreeNode.nextMessages[0].nextMessages[0].message.Message.(map[string]interface{})
					test2 := test["action"].(map[string]interface{})
					test3 := test2["task"].(map[string]interface{})
					test3["taskId"] = strings.Split(receivedMessageDecoded.String(), "\"")[1]
				}
				if err != nil {
					return err
				}
			}

			var found bool
			for _, message := range messageTreeNode.nextMessages {
				if message.message.MessageOrigin == internal.ClientMessage && message.called != true {
					// found the matching message so recurse deeper into the tree
					message.parent = messageTreeNode
					messageTreeNode = message
					found = true
					break
				}
			}

			if !found {
				return status.Errorf(codes.Unavailable, "no matching saved responses for method %s and message", info.FullMethod)
			}
		}

		if len(messageTreeNode.nextMessages) == 0 {
			messageTreeNode.called = true
			// end of the exchange
			return nil
		}
	}
}
