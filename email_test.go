package utils

import (
	"github.com/Laisky/zap"
)

func ExampleMail() {
	sender := NewMail("smtp_host", 53)
	if err := sender.Send(
		"fromAddr",
		"toAddr",
		"frName",
		"toName",
		"Title",
		"Content",
	); err != nil {
		Logger.Error("try to send email got error", zap.Error(err))
	}
}
