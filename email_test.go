package utils_test

import (
	"testing"

	utils "github.com/Laisky/go-utils"
)

func TestSend(t *testing.T) {
	sender := utils.NewMail(utils.Settings.GetString("email.host"), utils.Settings.GetInt("email.port"))
	sender.Login(utils.Settings.GetString("email.username"), utils.Settings.GetString("email.password"))
	err := sender.Send(
		"ramjet@pateo.com.cn",
		"zhonghuacai@pateo.com.cn",
		"Go-Ramjet",
		"Laisky Cai",
		"Go-Ramjet Alert",
		"alert text",
	)
	if err != nil {
		t.Errorf("got error %+v", err)
	}
}

func init() {
	utils.SetupLogger("debug")
	utils.Settings.Set("debug", true)

	utils.Settings.Setup("/Users/laisky/repo/pateo/configs/go-ramjet")
}
