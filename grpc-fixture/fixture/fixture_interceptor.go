package fixture

import (
	"strings"
	"sync"
	"time"

	"github.com/bradleyjkemp/grpc-tools/internal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CalledState struct {
	globalCalled bool
	mux          sync.Mutex
}

func (locker *CalledState) ChangeState(newState bool) {
	locker.mux.Lock()
	locker.globalCalled = newState
	locker.mux.Unlock()
}

func (locker *CalledState) CheckState() bool {
	locker.mux.Lock()
	defer locker.mux.Unlock()
	return locker.globalCalled
}

// fixtureInterceptor implements a gRPC.StreamingServerInterceptor that replays saved responses
func (f fixtureStruct) intercept(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, _ grpc.StreamHandler) error {
	messageTreeNode := f.fixture[info.FullMethod]
	var taskId = ""
	if messageTreeNode == nil {
		return status.Error(codes.Unavailable, "no saved responses found for method "+info.FullMethod)
	}
	gc := CalledState{
		globalCalled: false,
	}
	for {
		if !gc.CheckState() {
			for _, message := range messageTreeNode.nextMessages {
				if !message.called {
					if message.message.MessageOrigin == internal.ServerMessage {
						var (
							msgBytes = message.message.RawMessage
							err      error
						)

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
						sendMsgErr := ss.SendMsg(msgBytes)
						if sendMsgErr != nil {
							return sendMsgErr
						}

						// recurse deeper into the tree
						message.called = true
						gc.ChangeState(true)
						messageTreeNode = message
						if len(messageTreeNode.nextMessages) == 0 {
							messageTreeNode.called = true
							// end of the exchange
							return nil
						}
						// found the first server message so break
						break
					} else {
						// wait for a client message and then proceed based on its contents
						var receivedMessage []byte
						err := ss.RecvMsg(&receivedMessage)
						if err != nil {
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
						gc.ChangeState(true)
						messageTreeNode = message
						break
					}
				}
			}
		}
	}
}
