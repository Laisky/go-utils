package utils_test

import (
	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func ExampleMail() {
	sender := utils.NewMail("smtp_host", 53)
	if err := sender.Send(
		"fromAddr",
		"toAddr",
		"frName",
		"toName",
		"Title",
		"Content",
	); err != nil {
		utils.Logger.Error("try to send email got error", zap.Error(err))
	}
}
