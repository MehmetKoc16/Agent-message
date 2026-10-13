package client

import "github.com/MehmetKoc16/Agent-message/internal/proto"

func protoMsg(from string, replyTo *int64) proto.Message {
	return proto.Message{ID: 5, From: from, To: "x", Type: "soru", Thread: "T-1", Turn: 1, MaxTurns: 20, Body: "b", ReplyTo: replyTo}
}
