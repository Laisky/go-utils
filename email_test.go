package utils_test

import (
	"testing"

	"github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
)

func TestSend(t *testing.T) {
	utils.Settings.Set("debug", true)
	utils.Settings.Setup("/Users/laisky/repo/pateo/configs/go-ramjet")

	sender := utils.NewMail(utils.Settings.GetString("email.host"), utils.Settings.GetInt("email.port"))
	sender.Login(utils.Settings.GetString("email.username"), utils.Settings.GetString("email.password"))
	origDry := utils.Settings.GetBool("dry")
	utils.Settings.Set("dry", true)
	if err := sender.Send(
		"ramjet@pateo.com.cn",
		"zhonghuacai@pateo.com.cn",
		"Go-Ramjet",
		"Laisky Cai",
		"Go-Ramjet Alert",
		"alert text",
	); err != nil {
		t.Errorf("got error %+v", err)
	}
	utils.Settings.Set("dry", origDry)
}

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
