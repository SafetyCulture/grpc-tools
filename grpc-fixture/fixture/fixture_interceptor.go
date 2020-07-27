package fixture

import (
	"log"
	"strings"
	"time"

	"github.com/bradleyjkemp/grpc-tools/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fixtureInterceptor implements a gRPC.StreamingServerInterceptor that replays saved responses
func (f fixtureStruct) intercept(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, _ grpc.StreamHandler) error {
	log.Print("intercept for " + info.FullMethod)
	messageTreeNode := f.fixture[info.FullMethod]
	var taskId = ""
	if messageTreeNode == nil {
		log.Print("ERROR - No saved responses found for method "+info.FullMethod)
		return status.Error(codes.Unavailable, "no saved responses found for method "+info.FullMethod)
	}

	for {
		var clientOrServerWasCalled  = false
		for _, message := range messageTreeNode.nextMessages {
			log.Print("Process message " + info.FullMethod)
			if !message.called {
				clientOrServerWasCalled = true
				if message.message.MessageOrigin == internal.ClientMessage {
					log.Print("Process Client message " + info.FullMethod)
					// wait for a client message and then proceed based on its contents
					var receivedMessage []byte
					err := ss.RecvMsg(&receivedMessage)
					if err != nil {
						log.Print("Error to process Client request " + info.FullMethod)
						return err
					}

					if info.FullMethod == "/s12.tasks.v1.ActionsService/GetAction" {
						receivedMessageStructure := internal.Message{
							MessageOrigin: internal.ClientMessage,
							RawMessage:    receivedMessage,
							Message:       nil,
							Timestamp:     time.Time{},
						}
						receivedMessageDecoded, decodeErr := f.decoder.Decode(info.FullMethod, &receivedMessageStructure)
						if decodeErr != nil {
							return decodeErr
						}
						//Don't have access to values, this is why use split
						taskId = strings.Split(receivedMessageDecoded.String(), "\"")[1]
					}
					if info.FullMethod == "/s12.tasks.v1.IncidentsService/GetIncident" {
						receivedMessageStructure := internal.Message{
							MessageOrigin: internal.ClientMessage,
							RawMessage:    receivedMessage,
							Message:       nil,
							Timestamp:     time.Time{},
						}
						receivedMessageDecoded, decodeErr := f.decoder.Decode(info.FullMethod, &receivedMessageStructure)
						if decodeErr != nil {
							return decodeErr
						}
						//Don't have access to values, this is why use split
						taskId = strings.Split(receivedMessageDecoded.String(), "\"")[1]
					}
					// found the matching message so recurse deeper into the tree
					message.called = true
					messageTreeNode = message
				} else {
					var (
						msgBytes = message.message.RawMessage
						err      error
					)
					log.Print("Process Server message " + info.FullMethod)

					if info.FullMethod == "/s12.tasks.v1.ActionsService/GetAction" {
						//Override the action ID from dump on ID that's coming from client
						level1Nesting, ok := message.message.Message.(map[string]interface{})
						if !ok {
							return nil
						}
						actionMap, ok := level1Nesting["action"].(map[string]interface{})
						if !ok {
							return nil
						}
						taskMap, ok := actionMap["task"].(map[string]interface{})
						if !ok {
							return nil
						}
						taskMap["taskId"] = taskId

						msgBytes, err = f.encoder.Encode(info.FullMethod, message.message)
						if err != nil {
							return err
						}
					}
					if info.FullMethod == "/s12.tasks.v1.IncidentsService/GetIncident" {
						level1Nesting, ok := message.message.Message.(map[string]interface{})
						if !ok {
							return nil
						}
						incidentMap, ok := level1Nesting["incident"].(map[string]interface{})
						if !ok {
							return nil
						}
						taskMap, ok := incidentMap["task"].(map[string]interface{})
						if !ok {
							return nil
						}
						taskMap["taskId"] = taskId

						msgBytes, err = f.encoder.Encode(info.FullMethod, message.message)
						if err != nil {
							return err
						}
					}
					log.Print("Server response for " + info.FullMethod)
					message.called = true
					sendMsgErr := ss.SendMsg(msgBytes)
					//gc.ChangeState(false)
					if sendMsgErr != nil {
						return sendMsgErr
					}
				}
				//recurse deeper into the tree
				messageTreeNode = message
				if len(messageTreeNode.nextMessages) == 0 {
					messageTreeNode.called = true
					// end of the exchange
					return nil
				}
				// found the first server message so break
				break
			}
		}
		if !clientOrServerWasCalled {
			log.Print("ERROR - No saved NOT Called responses found for method "+info.FullMethod)
			return status.Error(codes.Unavailable, "no saved NOT Called responses found for method "+info.FullMethod)
		}
	}
}
