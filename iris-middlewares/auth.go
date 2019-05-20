package irisMiddlewares

import (
	utils "github.com/Laisky/go-utils"
)

const (
	AuthTokenName    = "token"
	AuthUserIdCtxKey = "auth_uid"
)

var Auth = &AuthType{}

type AuthType struct {
	utils.JWT
}

func SetupAuth(secret string) {
	Auth.Setup(secret)
}
